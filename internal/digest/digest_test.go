package digest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// seedDigestFixtures creates: one verified human, two agents with posts
// this week (agentA: 3 posts scores 50/30/10, agentB: 1 post score 20),
// the human following both agents, and provenance stats for agentA only.
// Returns (pool, humanID, agentAID, agentBID).
func seedDigestFixtures(t *testing.T) (*pgxpool.Pool, string, string, string) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"follows", "agent_provenance_stats", "api_keys", "provenances",
		"posts", "communities", "agent_identities", "human_users", "participants",
	)

	ctx := context.Background()
	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	followRepo := repository.NewFollowRepo(pool)
	statsRepo := repository.NewProvenanceStatsRepo(pool)

	human, err := pRepo.CreateHuman(ctx, &models.HumanUser{
		Participant:       models.Participant{DisplayName: "Reader"},
		Email:             "reader@example.com",
		PasswordHash:      "x",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	})
	if err != nil {
		t.Fatalf("CreateHuman: %v", err)
	}
	// Digest recipients require is_verified = TRUE.
	if _, err := pool.Exec(ctx,
		`UPDATE participants SET is_verified = TRUE WHERE id = $1`, human.ID); err != nil {
		t.Fatalf("verify human: %v", err)
	}

	mkAgent := func(name string) string {
		a, err := pRepo.CreateAgent(ctx, &models.AgentIdentity{
			Participant:       models.Participant{DisplayName: name},
			OwnerID:           human.ID,
			ModelProvider:     "openai",
			ModelName:         "gpt-4",
			MaxRPM:            60,
			ProtocolType:      models.ProtocolREST,
			HeartbeatInterval: 300,
			Capabilities:      []string{"read"},
		})
		if err != nil {
			t.Fatalf("CreateAgent %s: %v", name, err)
		}
		return a.ID
	}
	agentA := mkAgent("Curie Test")
	agentB := mkAgent("Orpheus Test")

	comm, err := cRepo.Create(ctx, &models.Community{
		Name: "digest-test", Slug: "digest-test", Description: "t", CreatedBy: human.ID,
	})
	if err != nil {
		t.Fatalf("Create community: %v", err)
	}

	mkPost := func(author, title string, score int) {
		p, err := postRepo.Create(ctx, &models.Post{
			CommunityID: comm.ID,
			AuthorID:    author,
			AuthorType:  models.ParticipantAgent,
			Title:       title,
			Body:        "body",
			PostType:    models.PostTypeText,
		})
		if err != nil {
			t.Fatalf("Create post %q: %v", title, err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE posts SET vote_score = $1 WHERE id = $2`, score, p.ID); err != nil {
			t.Fatalf("set vote_score: %v", err)
		}
	}
	mkPost(agentA, "A best post", 50)
	mkPost(agentA, "A second post", 30)
	mkPost(agentA, "A third post", 10)
	mkPost(agentB, "B only post", 20)

	if err := followRepo.Follow(ctx, human.ID, agentA); err != nil {
		t.Fatalf("Follow agentA: %v", err)
	}
	if err := followRepo.Follow(ctx, human.ID, agentB); err != nil {
		t.Fatalf("Follow agentB: %v", err)
	}

	// Provenance stats for agentA only (above the >=5 posts threshold).
	if err := statsRepo.Upsert(ctx, models.AgentProvenanceStats{
		AgentID: agentA, PostsCounted: 12, AvgSourcesPerPost: 1.1,
		PrimarySourcePct: 0.84, BeatConsistencyPct: 1, CadencePerWeek: 12,
	}); err != nil {
		t.Fatalf("Upsert stats: %v", err)
	}

	return pool, human.ID, agentA, agentB
}

func TestFetchVoiceSections(t *testing.T) {
	pool, humanID, agentA, agentB := seedDigestFixtures(t)

	period := digestTestPeriod(CadenceWeekly)
	sections, err := fetchVoiceSections(context.Background(), pool, []string{humanID}, period.Start, period.End)
	if err != nil {
		t.Fatalf("fetchVoiceSections: %v", err)
	}
	voices := sections[humanID]
	if len(voices) != 2 {
		t.Fatalf("want 2 voices, got %d: %+v", len(voices), voices)
	}
	// Ordered by best post score desc: agentA (50) before agentB (20).
	if voices[0].AgentID != agentA || voices[1].AgentID != agentB {
		t.Fatalf("voice order wrong: %+v", voices)
	}
	// Top 2 per voice only.
	if len(voices[0].Posts) != 2 {
		t.Fatalf("want top 2 posts for agentA, got %d", len(voices[0].Posts))
	}
	if voices[0].Posts[0].Title != "A best post" || voices[0].Posts[1].Title != "A second post" {
		t.Fatalf("agentA posts wrong: %+v", voices[0].Posts)
	}
	if voices[0].PostsInPeriod != 3 {
		t.Fatalf("agentA posts_in_period: want 3, got %d", voices[0].PostsInPeriod)
	}
	// Provenance byline stat: present for agentA, absent for agentB.
	if voices[0].VerifiedPct == nil || *voices[0].VerifiedPct < 0.83 || *voices[0].VerifiedPct > 0.85 {
		t.Fatalf("agentA VerifiedPct: want ~0.84, got %v", voices[0].VerifiedPct)
	}
	if voices[1].VerifiedPct != nil {
		t.Fatalf("agentB VerifiedPct: want nil, got %v", *voices[1].VerifiedPct)
	}
}

func TestFetchVoiceSections_NoFollows(t *testing.T) {
	pool, _, _, _ := seedDigestFixtures(t)
	period := digestTestPeriod(CadenceWeekly)
	sections, err := fetchVoiceSections(context.Background(), pool,
		[]string{"00000000-0000-0000-0000-000000000001"}, period.Start, period.End)
	if err != nil {
		t.Fatalf("fetchVoiceSections: %v", err)
	}
	if len(sections) != 0 {
		t.Fatalf("want empty map for non-follower, got %+v", sections)
	}
}

func TestFetchRecipientsSelectsOnlyRequestedCadence(t *testing.T) {
	pool, _, _, _ := seedDigestFixtures(t)
	ctx := context.Background()
	pRepo := repository.NewParticipantRepo(pool)

	createRecipient := func(name, email string, cadence Cadence) string {
		t.Helper()
		human, err := pRepo.CreateHuman(ctx, &models.HumanUser{
			Participant:       models.Participant{DisplayName: name},
			Email:             email,
			PasswordHash:      "x",
			PreferredLanguage: "en",
			NotificationPrefs: "{}",
		})
		if err != nil {
			t.Fatalf("CreateHuman %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE participants SET is_verified = TRUE WHERE id = $1`, human.ID); err != nil {
			t.Fatalf("verify recipient %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE human_users SET digest_frequency = $2 WHERE participant_id = $1`, human.ID, cadence); err != nil {
			t.Fatalf("set cadence for recipient %s: %v", name, err)
		}
		return human.ID
	}

	dailyID := createRecipient("Daily Reader", "daily@example.com", CadenceDaily)
	_ = createRecipient("Off Reader", "off@example.com", CadenceOff)

	daily, err := fetchRecipients(ctx, pool, CadenceDaily)
	if err != nil {
		t.Fatalf("fetch daily recipients: %v", err)
	}
	if len(daily) != 1 || daily[0].ParticipantID != dailyID {
		t.Fatalf("daily recipients=%+v, want only %s", daily, dailyID)
	}

	weekly, err := fetchRecipients(ctx, pool, CadenceWeekly)
	if err != nil {
		t.Fatalf("fetch weekly recipients: %v", err)
	}
	if len(weekly) != 1 || weekly[0].Email != "reader@example.com" {
		t.Fatalf("weekly recipients=%+v, want only reader@example.com", weekly)
	}
}

func TestPeriodsDueAt09UTCIncludeDailyAndMondayWeekly(t *testing.T) {
	tuesday := time.Date(2026, time.August, 11, 9, 0, 0, 0, time.UTC)
	periods := PeriodsDueAt(tuesday)
	if len(periods) != 1 || periods[0].Cadence != CadenceDaily {
		t.Fatalf("Tuesday periods=%+v, want daily only", periods)
	}
	if got := periods[0].Start; !got.Equal(tuesday.Add(-24 * time.Hour)) {
		t.Fatalf("daily period start=%v", got)
	}

	monday := time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC)
	periods = PeriodsDueAt(monday)
	if len(periods) != 2 || periods[0].Cadence != CadenceDaily || periods[1].Cadence != CadenceWeekly {
		t.Fatalf("Monday periods=%+v, want daily then weekly", periods)
	}
	if got := periods[1].Start; !got.Equal(monday.Add(-7 * 24 * time.Hour)) {
		t.Fatalf("weekly period start=%v", got)
	}
}

func TestDigestScheduleBoundaries(t *testing.T) {
	for _, test := range []struct {
		name       string
		now        time.Time
		nextDaily  time.Time
		nextWeekly time.Time
		lastWeekly time.Time
	}{
		{
			name:       "before Monday tick",
			now:        time.Date(2026, time.August, 10, 8, 30, 0, 0, time.UTC),
			nextDaily:  time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC),
			nextWeekly: time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC),
			lastWeekly: time.Date(2026, time.August, 3, 9, 0, 0, 0, time.UTC),
		},
		{
			name:       "after Monday tick",
			now:        time.Date(2026, time.August, 10, 9, 30, 0, 0, time.UTC),
			nextDaily:  time.Date(2026, time.August, 11, 9, 0, 0, 0, time.UTC),
			nextWeekly: time.Date(2026, time.August, 17, 9, 0, 0, 0, time.UTC),
			lastWeekly: time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := NextRunAt09UTC(test.now); !got.Equal(test.nextDaily) {
				t.Fatalf("next daily=%v, want %v", got, test.nextDaily)
			}
			if got := NextMondayAt09UTC(test.now); !got.Equal(test.nextWeekly) {
				t.Fatalf("next weekly=%v, want %v", got, test.nextWeekly)
			}
			if got := MostRecentMondayAt09UTC(test.now); !got.Equal(test.lastWeekly) {
				t.Fatalf("last weekly=%v, want %v", got, test.lastWeekly)
			}
		})
	}
}

// TestFetchVoiceSections_VoiceCap asserts that fetchVoiceSections returns at
// most maxVoicesPerDigest (8) voices even when more are followed. The 9 extra
// agents added here all have scores 100–108 (higher than agentA's 50 and
// agentB's 20), so they fill the 8 slots and the two lowest-scored voices
// (agentA at 50 and agentB at 20) are both pushed out.
func TestFetchVoiceSections_VoiceCap(t *testing.T) {
	pool, humanID, _, agentB := seedDigestFixtures(t)

	ctx := context.Background()
	pRepo := repository.NewParticipantRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	followRepo := repository.NewFollowRepo(pool)

	// seedDigestFixtures already created a community; reuse it.
	var commID string
	if err := pool.QueryRow(ctx, `SELECT id FROM communities WHERE slug = 'digest-test'`).Scan(&commID); err != nil {
		t.Fatalf("find community: %v", err)
	}

	// Create 9 extra agents, each with one post scored 100..108.
	// All scores exceed agentA (50) and agentB (20), so these 9 agents rank
	// above both originals and the cap will exclude the lower-scored voices.
	for i := 0; i < 9; i++ {
		score := 100 + i
		a, err := pRepo.CreateAgent(ctx, &models.AgentIdentity{
			Participant:       models.Participant{DisplayName: fmt.Sprintf("ExtraAgent%d", i)},
			OwnerID:           humanID,
			ModelProvider:     "openai",
			ModelName:         "gpt-4",
			MaxRPM:            60,
			ProtocolType:      models.ProtocolREST,
			HeartbeatInterval: 300,
			Capabilities:      []string{"read"},
		})
		if err != nil {
			t.Fatalf("CreateAgent extra %d: %v", i, err)
		}
		p, err := postRepo.Create(ctx, &models.Post{
			CommunityID: commID,
			AuthorID:    a.ID,
			AuthorType:  models.ParticipantAgent,
			Title:       fmt.Sprintf("Extra post %d", i),
			Body:        "body",
			PostType:    models.PostTypeText,
		})
		if err != nil {
			t.Fatalf("Create post extra %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE posts SET vote_score = $1 WHERE id = $2`, score, p.ID); err != nil {
			t.Fatalf("set vote_score extra %d: %v", i, err)
		}
		if err := followRepo.Follow(ctx, humanID, a.ID); err != nil {
			t.Fatalf("Follow extra agent %d: %v", i, err)
		}
	}

	period := digestTestPeriod(CadenceWeekly)
	sections, err := fetchVoiceSections(ctx, pool, []string{humanID}, period.Start, period.End)
	if err != nil {
		t.Fatalf("fetchVoiceSections: %v", err)
	}

	voices := sections[humanID]
	if len(voices) != maxVoicesPerDigest {
		t.Fatalf("want %d voices (cap), got %d", maxVoicesPerDigest, len(voices))
	}

	// agentB (best score 20) must NOT appear — it is outranked by all 9 extra
	// agents (100–108) and should have been cut by the cap.
	for _, v := range voices {
		if v.AgentID == agentB {
			t.Fatalf("agentB (score 20) should be excluded by the %d-voice cap, but it appears in the result", maxVoicesPerDigest)
		}
	}
}

func TestRenderDigest_WithVoices(t *testing.T) {
	r := Recipient{ParticipantID: "p1", Email: "x@example.com", DisplayName: "Reader"}
	posts := []TopPost{{ID: "g1", Title: "Global <Top>", CommunitySlug: "ai", AuthorName: "Nebula", VoteScore: 9, CommentCount: 2}}
	pct := 0.84
	voices := []VoiceSection{{
		AgentID: "a1", AgentName: "Curie & Co", PostsInPeriod: 12, VerifiedPct: &pct,
		Posts: []VoicePost{
			{ID: "v1", Title: "Voice <b>post</b> one", VoteScore: 50, CommentCount: 3},
			{ID: "v2", Title: "Voice post two", VoteScore: 30, CommentCount: 1},
		},
	}}

	html, plain := renderDigest(r, posts, voices, "https://example.com", "tok", CadenceWeekly)

	for _, want := range []string{
		"From your voices",
		"Curie &amp; Co", // escaped agent name
		"12 posts this week",
		"84% verified sources",
		"Voice &lt;b&gt;post&lt;/b&gt; one", // escaped title
		"https://example.com/post/v1",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q", want)
		}
	}
	for _, want := range []string{"From your voices", "Curie & Co", "Voice <b>post</b> one"} {
		if !strings.Contains(plain, want) {
			t.Errorf("plain missing %q", want)
		}
	}
}

func TestRenderDigest_NoVoices_Unchanged(t *testing.T) {
	r := Recipient{ParticipantID: "p1", Email: "x@example.com", DisplayName: "Reader"}
	posts := []TopPost{{ID: "g1", Title: "Global", CommunitySlug: "ai", AuthorName: "Nebula", VoteScore: 9, CommentCount: 2}}

	html, plain := renderDigest(r, posts, nil, "https://example.com", "tok", CadenceWeekly)

	if strings.Contains(html, "From your voices") || strings.Contains(plain, "From your voices") {
		t.Fatal("nil voices must not render the section")
	}
}

func TestRenderDailyDigestUsesDailyCopy(t *testing.T) {
	r := Recipient{ParticipantID: "p1", Email: "x@example.com", DisplayName: "Reader"}
	posts := []TopPost{{ID: "g1", Title: "Global", CommunitySlug: "ai", AuthorName: "Nebula"}}
	voices := []VoiceSection{{
		AgentID: "a1", AgentName: "Curie", PostsInPeriod: 1,
		Posts: []VoicePost{{ID: "v1", Title: "Voice post"}},
	}}

	html, plain := renderDigest(r, posts, voices, "https://example.com", "tok", CadenceDaily)
	for _, body := range []string{html, plain} {
		if !strings.Contains(body, "today") {
			t.Fatalf("daily digest missing daily copy: %q", body)
		}
		if strings.Contains(body, "this week") {
			t.Fatalf("daily digest contains weekly copy: %q", body)
		}
	}
}

func TestRenderDigest_VoiceWithoutStat(t *testing.T) {
	r := Recipient{ParticipantID: "p1", Email: "x@example.com", DisplayName: "Reader"}
	posts := []TopPost{{ID: "g1", Title: "Global", CommunitySlug: "ai", AuthorName: "Nebula", VoteScore: 9, CommentCount: 2}}
	voices := []VoiceSection{{
		AgentID: "a2", AgentName: "Orpheus", PostsInPeriod: 1, VerifiedPct: nil,
		Posts: []VoicePost{{ID: "v3", Title: "Solo", VoteScore: 20, CommentCount: 0}},
	}}

	html, _ := renderDigest(r, posts, voices, "https://example.com", "tok", CadenceWeekly)

	if strings.Contains(html, "verified sources") {
		t.Fatal("nil VerifiedPct must omit the stat clause")
	}
	if !strings.Contains(html, "1 post this week") {
		t.Error("singular 'post' expected for count of 1")
	}
}

// captureSender records every email instead of sending.
type captureSender struct {
	emails []capturedEmail
}
type capturedEmail struct {
	To, Subject, HTML, Plain string
}

func (c *captureSender) Send(to, toName, subject, htmlBody, plainText string) error {
	c.emails = append(c.emails, capturedEmail{To: to, Subject: subject, HTML: htmlBody, Plain: plainText})
	return nil
}

type deliveryCall struct {
	ID        string
	FirstSent time.Time
	To        string
	Subject   string
	HTML      string
	Plain     string
}

type ledgerCaptureSender struct {
	mu            sync.Mutex
	calls         []deliveryCall
	failRemaining map[string]int
	started       chan struct{}
	release       chan struct{}
	startOnce     sync.Once
}

func (s *ledgerCaptureSender) Send(to, toName, subject, htmlBody, plainText string) error {
	return fmt.Errorf("digest did not use idempotent provider send for %s", to)
}

func (s *ledgerCaptureSender) SendIdempotent(
	deliveryID string,
	firstSent time.Time,
	to, toName, subject, htmlBody, plainText string,
) error {
	s.mu.Lock()
	s.calls = append(s.calls, deliveryCall{
		ID: deliveryID, FirstSent: firstSent, To: to,
		Subject: subject, HTML: htmlBody, Plain: plainText,
	})
	remaining := s.failRemaining[to]
	if remaining > 0 {
		s.failRemaining[to] = remaining - 1
	}
	s.mu.Unlock()

	if s.started != nil {
		s.startOnce.Do(func() { close(s.started) })
		<-s.release
	}
	if remaining > 0 {
		return fmt.Errorf("temporary provider failure for %s", to)
	}
	return nil
}

func (s *ledgerCaptureSender) callsFor(to string) []deliveryCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	var calls []deliveryCall
	for _, call := range s.calls {
		if call.To == to {
			calls = append(calls, call)
		}
	}
	return calls
}

func digestTestPeriod(cadence Cadence) Period {
	end := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	return PeriodEndingAt(cadence, end)
}

func TestRunPeriodElectsOneOfTwoConcurrentSchedulers(t *testing.T) {
	pool, recipientID, _, _ := seedDigestFixtures(t)
	ctx := context.Background()
	pRepo := repository.NewParticipantRepo(pool)
	second, err := pRepo.CreateHuman(ctx, &models.HumanUser{
		Participant:       models.Participant{DisplayName: "Second Reader"},
		Email:             "second@example.com",
		PasswordHash:      "x",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	})
	if err != nil {
		t.Fatalf("CreateHuman second: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE participants SET is_verified = TRUE WHERE id = $1`, second.ID); err != nil {
		t.Fatalf("verify second recipient: %v", err)
	}
	sender := &ledgerCaptureSender{
		failRemaining: map[string]int{},
		started:       make(chan struct{}),
		release:       make(chan struct{}),
	}
	period := digestTestPeriod(CadenceWeekly)
	cfg := Config{Pool: pool, Sender: sender, SiteURL: "https://example.com", UnsubKey: "k"}
	type runResult struct {
		sent int
		err  error
	}
	firstResult := make(chan runResult, 1)
	go func() {
		sent, err := RunPeriod(context.Background(), cfg, period)
		firstResult <- runResult{sent: sent, err: err}
	}()

	select {
	case <-sender.started:
	case <-time.After(5 * time.Second):
		t.Fatal("first scheduler did not reach the provider")
	}
	secondSent, secondErr := RunPeriod(context.Background(), cfg, period)
	close(sender.release)
	first := <-firstResult
	if first.err != nil || first.sent != 2 {
		t.Fatalf("first scheduler sent=%d err=%v, want 2 nil", first.sent, first.err)
	}
	if secondErr != nil || secondSent != 0 {
		t.Fatalf("second scheduler sent=%d err=%v, want 0 nil", secondSent, secondErr)
	}
	if calls := sender.callsFor("reader@example.com"); len(calls) != 1 {
		t.Fatalf("provider calls=%+v, want exactly one", calls)
	}
	if calls := sender.callsFor("second@example.com"); len(calls) != 1 {
		t.Fatalf("second recipient provider calls=%+v, want exactly one", calls)
	}
	for _, id := range []string{recipientID, second.ID} {
		var status string
		var attempts int
		if err := pool.QueryRow(context.Background(), `
			SELECT status, attempt_count
			FROM digest_deliveries
			WHERE recipient_id = $1 AND cadence = $2 AND period_start = $3`,
			id, period.Cadence, period.Start).Scan(&status, &attempts); err != nil {
			t.Fatalf("load delivery ledger for %s: %v", id, err)
		}
		if status != "sent" || attempts != 1 {
			t.Fatalf("delivery %s status=%q attempts=%d, want sent/1", id, status, attempts)
		}
	}
}

func TestRunPeriodRetriesOnlyFailedRecipientsWithStableDeliveryID(t *testing.T) {
	pool, readerID, _, _ := seedDigestFixtures(t)
	ctx := context.Background()
	pRepo := repository.NewParticipantRepo(pool)
	second, err := pRepo.CreateHuman(ctx, &models.HumanUser{
		Participant:       models.Participant{DisplayName: "Second Reader"},
		Email:             "second@example.com",
		PasswordHash:      "x",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	})
	if err != nil {
		t.Fatalf("CreateHuman second: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE participants SET is_verified = TRUE WHERE id = $1`, second.ID); err != nil {
		t.Fatalf("verify second recipient: %v", err)
	}

	sender := &ledgerCaptureSender{failRemaining: map[string]int{"reader@example.com": 1}}
	period := digestTestPeriod(CadenceWeekly)
	cfg := Config{Pool: pool, Sender: sender, SiteURL: "https://example.com", UnsubKey: "k"}

	firstSent, firstErr := RunPeriod(ctx, cfg, period)
	if firstErr == nil || firstSent != 1 {
		t.Fatalf("first run sent=%d err=%v, want one success and a provider error", firstSent, firstErr)
	}
	if _, err := pool.Exec(ctx, `UPDATE posts SET title = 'Changed after first attempt'`); err != nil {
		t.Fatalf("change digest content before retry: %v", err)
	}
	sender.started = make(chan struct{})
	sender.release = make(chan struct{})
	sender.startOnce = sync.Once{}
	type retryResult struct {
		sent int
		err  error
	}
	winnerResult := make(chan retryResult, 1)
	go func() {
		sent, err := RunPeriod(context.Background(), cfg, period)
		winnerResult <- retryResult{sent: sent, err: err}
	}()
	select {
	case <-sender.started:
	case <-time.After(5 * time.Second):
		t.Fatal("retrying scheduler did not reach the provider")
	}
	loserSent, loserErr := RunPeriod(ctx, cfg, period)
	close(sender.release)
	winner := <-winnerResult
	if winner.err != nil || winner.sent != 1 {
		t.Fatalf("winning retry sent=%d err=%v, want one recovered delivery", winner.sent, winner.err)
	}
	if loserErr != nil || loserSent != 0 {
		t.Fatalf("concurrent retry sent=%d err=%v, want leader-election no-op", loserSent, loserErr)
	}
	thirdSent, thirdErr := RunPeriod(ctx, cfg, period)
	if thirdErr != nil || thirdSent != 0 {
		t.Fatalf("completed rerun sent=%d err=%v, want idempotent no-op", thirdSent, thirdErr)
	}

	readerCalls := sender.callsFor("reader@example.com")
	if len(readerCalls) != 2 {
		t.Fatalf("failed recipient calls=%+v, want initial attempt plus retry", readerCalls)
	}
	if readerCalls[0].ID == "" || readerCalls[0].ID != readerCalls[1].ID {
		t.Fatalf("retry delivery IDs=%q and %q, want one stable non-empty ID", readerCalls[0].ID, readerCalls[1].ID)
	}
	if !readerCalls[0].FirstSent.Equal(readerCalls[1].FirstSent) {
		t.Fatalf("retry first-sent values=%v and %v, want stable value", readerCalls[0].FirstSent, readerCalls[1].FirstSent)
	}
	if readerCalls[0].Subject != readerCalls[1].Subject ||
		readerCalls[0].HTML != readerCalls[1].HTML ||
		readerCalls[0].Plain != readerCalls[1].Plain {
		t.Fatal("retry regenerated a different provider payload for the same delivery ID")
	}
	if strings.Contains(readerCalls[1].HTML, "Changed after first attempt") {
		t.Fatal("retry used mutable post content instead of the persisted delivery payload")
	}
	if calls := sender.callsFor("second@example.com"); len(calls) != 1 {
		t.Fatalf("successful recipient calls=%+v, want no resend during retry", calls)
	}
	for _, want := range []struct {
		recipientID string
		attempts    int
	}{
		{recipientID: readerID, attempts: 2},
		{recipientID: second.ID, attempts: 1},
	} {
		var status string
		var attempts int
		var payloadCleared bool
		if err := pool.QueryRow(ctx, `
			SELECT status, attempt_count,
			       recipient_email IS NULL AND recipient_name IS NULL AND
			       subject IS NULL AND html_body IS NULL AND plain_text IS NULL AND
			       post_ids IS NULL
			FROM digest_deliveries
			WHERE recipient_id = $1 AND cadence = $2 AND period_start = $3`,
			want.recipientID, period.Cadence, period.Start).Scan(&status, &attempts, &payloadCleared); err != nil {
			t.Fatalf("load delivery ledger for %s: %v", want.recipientID, err)
		}
		if status != "sent" || attempts != want.attempts || !payloadCleared {
			t.Fatalf("delivery %s status=%q attempts=%d payload_cleared=%t, want sent/%d/true",
				want.recipientID, status, attempts, payloadCleared, want.attempts)
		}
	}
}

func TestRunPeriodCancelsFailedDeliveryWhenContentBecomesPrivate(t *testing.T) {
	pool, readerID, _, _ := seedDigestFixtures(t)
	ctx := context.Background()
	sender := &ledgerCaptureSender{failRemaining: map[string]int{"reader@example.com": 1}}
	period := digestTestPeriod(CadenceWeekly)
	cfg := Config{Pool: pool, Sender: sender, SiteURL: "https://example.com", UnsubKey: "k"}

	if sent, err := RunPeriod(ctx, cfg, period); err == nil || sent != 0 {
		t.Fatalf("first run sent=%d err=%v, want failed provider delivery", sent, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE posts SET quarantined = TRUE WHERE title = 'A best post'`); err != nil {
		t.Fatalf("quarantine referenced post: %v", err)
	}
	if sent, err := RunPeriod(ctx, cfg, period); err != nil || sent != 0 {
		t.Fatalf("retry after quarantine sent=%d err=%v, want canceled no-op", sent, err)
	}
	if calls := sender.callsFor("reader@example.com"); len(calls) != 1 {
		t.Fatalf("provider calls=%+v, want no retry after content became private", calls)
	}
	assertCanceledDeliveryPayloadCleared(t, pool, readerID, period)
}

func TestRunPeriodCancelsFailedDeliveryWhenRecipientOptsOut(t *testing.T) {
	pool, readerID, _, _ := seedDigestFixtures(t)
	ctx := context.Background()
	sender := &ledgerCaptureSender{failRemaining: map[string]int{"reader@example.com": 1}}
	period := digestTestPeriod(CadenceWeekly)
	cfg := Config{Pool: pool, Sender: sender, SiteURL: "https://example.com", UnsubKey: "k"}

	if sent, err := RunPeriod(ctx, cfg, period); err == nil || sent != 0 {
		t.Fatalf("first run sent=%d err=%v, want failed provider delivery", sent, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE human_users SET digest_frequency = 'off' WHERE participant_id = $1`, readerID); err != nil {
		t.Fatalf("disable recipient digest: %v", err)
	}
	if sent, err := RunPeriod(ctx, cfg, period); err != nil || sent != 0 {
		t.Fatalf("retry after opt-out sent=%d err=%v, want canceled no-op", sent, err)
	}
	if calls := sender.callsFor("reader@example.com"); len(calls) != 1 {
		t.Fatalf("provider calls=%+v, want no retry after recipient opted out", calls)
	}
	assertCanceledDeliveryPayloadCleared(t, pool, readerID, period)
}

func assertCanceledDeliveryPayloadCleared(
	t *testing.T,
	pool *pgxpool.Pool,
	recipientID string,
	period Period,
) {
	t.Helper()
	var status string
	var payloadCleared bool
	if err := pool.QueryRow(context.Background(), `
		SELECT status,
		       recipient_email IS NULL AND recipient_name IS NULL AND
		       subject IS NULL AND html_body IS NULL AND plain_text IS NULL AND
		       post_ids IS NULL
		FROM digest_deliveries
		WHERE recipient_id = $1 AND cadence = $2 AND period_start = $3`,
		recipientID, period.Cadence, period.Start).Scan(&status, &payloadCleared); err != nil {
		t.Fatalf("load canceled delivery: %v", err)
	}
	if status != "canceled" || !payloadCleared {
		t.Fatalf("delivery status=%q payload_cleared=%t, want canceled/true", status, payloadCleared)
	}
}

func TestDigestDeliveryLedgerHidesPendingPayloadFromRequestRole(t *testing.T) {
	pool, readerID, _, _ := seedDigestFixtures(t)
	sender := &ledgerCaptureSender{failRemaining: map[string]int{"reader@example.com": 1}}
	period := digestTestPeriod(CadenceWeekly)
	_, err := RunPeriod(context.Background(), Config{
		Pool: pool, Sender: sender, SiteURL: "https://example.com", UnsubKey: "k",
	}, period)
	if err == nil {
		t.Fatal("provider failure unexpectedly succeeded")
	}

	var pendingPayloads int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM digest_deliveries
		WHERE status = 'failed' AND recipient_email IS NOT NULL AND
		      html_body IS NOT NULL AND post_ids IS NOT NULL`).Scan(&pendingPayloads); err != nil {
		t.Fatalf("count service-visible pending payloads: %v", err)
	}
	if pendingPayloads != 1 {
		t.Fatalf("service-visible pending payloads=%d, want 1", pendingPayloads)
	}

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin request-role transaction: %v", err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(context.Background(), "SET LOCAL ROLE app_user"); err != nil {
		t.Fatalf("assume app_user: %v", err)
	}
	var requestVisible int
	if err := tx.QueryRow(context.Background(), `SELECT COUNT(*) FROM digest_deliveries`).Scan(&requestVisible); err != nil {
		t.Fatalf("query delivery ledger as context-free app_user: %v", err)
	}
	if requestVisible != 0 {
		t.Fatalf("context-free app_user saw %d service-only delivery rows", requestVisible)
	}
	if _, err := tx.Exec(context.Background(),
		`SELECT set_config('app.current_user_id', $1, true)`, readerID); err != nil {
		t.Fatalf("set request participant: %v", err)
	}
	if err := tx.QueryRow(context.Background(), `SELECT COUNT(*) FROM digest_deliveries`).Scan(&requestVisible); err != nil {
		t.Fatalf("query delivery ledger as app_user: %v", err)
	}
	if requestVisible != 0 {
		t.Fatalf("request role saw %d service-only delivery rows", requestVisible)
	}
}

func TestRunPeriodWithRetriesRecoversPartialProviderFailure(t *testing.T) {
	pool, _, _, _ := seedDigestFixtures(t)
	sender := &ledgerCaptureSender{failRemaining: map[string]int{"reader@example.com": 1}}
	period := digestTestPeriod(CadenceWeekly)
	cfg := Config{Pool: pool, Sender: sender, SiteURL: "https://example.com", UnsubKey: "k"}

	sent, err := RunPeriodWithRetries(context.Background(), cfg, period, 0)
	if err != nil || sent != 1 {
		t.Fatalf("retried run sent=%d err=%v, want one recovered delivery", sent, err)
	}
	calls := sender.callsFor("reader@example.com")
	if len(calls) != 2 || calls[0].ID != calls[1].ID {
		t.Fatalf("provider retry calls=%+v, want two calls with one delivery ID", calls)
	}
}

func TestRunPeriodPreviewDoesNotRecordDelivery(t *testing.T) {
	pool, _, _, _ := seedDigestFixtures(t)
	sender := &captureSender{}
	period := digestTestPeriod(CadenceWeekly)

	sent, err := RunPeriod(context.Background(), Config{
		Pool: pool, Sender: sender, SiteURL: "https://example.com", UnsubKey: "k", Preview: true,
	}, period)
	if err != nil || sent != 1 {
		t.Fatalf("preview sent=%d err=%v, want one rendered message", sent, err)
	}
	var deliveryCount int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM digest_deliveries`).Scan(&deliveryCount); err != nil {
		t.Fatalf("count preview deliveries: %v", err)
	}
	if deliveryCount != 0 {
		t.Fatalf("preview recorded %d deliveries, want zero", deliveryCount)
	}
}

func TestRunPeriodExcludesQuarantinedPosts(t *testing.T) {
	pool, _, _, _ := seedDigestFixtures(t)
	if _, err := pool.Exec(context.Background(), `
		UPDATE posts SET quarantined = TRUE WHERE title = 'A best post'`); err != nil {
		t.Fatalf("quarantine fixture post: %v", err)
	}
	sender := &captureSender{}

	sent, err := RunPeriod(context.Background(), Config{
		Pool: pool, Sender: sender, SiteURL: "https://example.com", UnsubKey: "k",
	}, digestTestPeriod(CadenceWeekly))
	if err != nil || sent != 1 {
		t.Fatalf("run digest sent=%d err=%v", sent, err)
	}
	if len(sender.emails) != 1 {
		t.Fatalf("emails=%d, want 1", len(sender.emails))
	}
	if strings.Contains(sender.emails[0].HTML, "A best post") ||
		strings.Contains(sender.emails[0].Plain, "A best post") {
		t.Fatal("quarantined post leaked into digest content")
	}
}

func TestRunPeriodPersonalizesPerRecipient(t *testing.T) {
	pool, _, _, _ := seedDigestFixtures(t)
	ctx := context.Background()

	// Second verified human with NO follows — must get the generic email.
	pRepo := repository.NewParticipantRepo(pool)
	lurker, err := pRepo.CreateHuman(ctx, &models.HumanUser{
		Participant:       models.Participant{DisplayName: "Lurker"},
		Email:             "lurker@example.com",
		PasswordHash:      "x",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	})
	if err != nil {
		t.Fatalf("CreateHuman lurker: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE participants SET is_verified = TRUE WHERE id = $1`, lurker.ID); err != nil {
		t.Fatalf("verify lurker: %v", err)
	}

	sender := &captureSender{}
	sent, err := RunPeriod(ctx,
		Config{Pool: pool, Sender: sender, SiteURL: "https://example.com", UnsubKey: "k"},
		digestTestPeriod(CadenceWeekly))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sent != 2 {
		t.Fatalf("want 2 sent, got %d", sent)
	}

	byTo := map[string]capturedEmail{}
	for _, e := range sender.emails {
		byTo[e.To] = e
	}

	reader := byTo["reader@example.com"]
	if !strings.Contains(reader.HTML, "From your voices") {
		t.Error("reader email missing voices section")
	}
	if !strings.Contains(reader.HTML, "Curie Test") || !strings.Contains(reader.HTML, "Orpheus Test") {
		t.Error("reader email missing followed voices")
	}
	// "A third post" (score 10) is in the global top-posts section AND is a
	// candidate for agentA's voice block — we only care that the voice cap
	// keeps it out of the "From your voices" section. Split on the header.
	voicesIdx := strings.Index(reader.HTML, "From your voices")
	if voicesIdx < 0 {
		t.Fatal("voices section not found (already caught above)")
	}
	if strings.Contains(reader.HTML[voicesIdx:], "A third post") {
		t.Error("top-2-per-voice cap violated: 3rd post leaked into voices section")
	}

	lurkerMail := byTo["lurker@example.com"]
	if strings.Contains(lurkerMail.HTML, "From your voices") {
		t.Error("zero-subscription recipient must get the unchanged email")
	}
}
