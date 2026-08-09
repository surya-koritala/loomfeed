package digest

import (
	"context"
	"fmt"
	"strings"
	"testing"

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

	sections, err := fetchVoiceSections(context.Background(), pool, []string{humanID})
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
	if voices[0].PostsThisWeek != 3 {
		t.Fatalf("agentA posts_this_week: want 3, got %d", voices[0].PostsThisWeek)
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
	sections, err := fetchVoiceSections(context.Background(), pool, []string{"00000000-0000-0000-0000-000000000001"})
	if err != nil {
		t.Fatalf("fetchVoiceSections: %v", err)
	}
	if len(sections) != 0 {
		t.Fatalf("want empty map for non-follower, got %+v", sections)
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

	sections, err := fetchVoiceSections(ctx, pool, []string{humanID})
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
		AgentID: "a1", AgentName: "Curie & Co", PostsThisWeek: 12, VerifiedPct: &pct,
		Posts: []VoicePost{
			{ID: "v1", Title: "Voice <b>post</b> one", VoteScore: 50, CommentCount: 3},
			{ID: "v2", Title: "Voice post two", VoteScore: 30, CommentCount: 1},
		},
	}}

	html, plain := renderDigest(r, posts, voices, "https://example.com", "tok")

	for _, want := range []string{
		"From your voices",
		"Curie &amp; Co",                     // escaped agent name
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

	html, plain := renderDigest(r, posts, nil, "https://example.com", "tok")

	if strings.Contains(html, "From your voices") || strings.Contains(plain, "From your voices") {
		t.Fatal("nil voices must not render the section")
	}
}

func TestRenderDigest_VoiceWithoutStat(t *testing.T) {
	r := Recipient{ParticipantID: "p1", Email: "x@example.com", DisplayName: "Reader"}
	posts := []TopPost{{ID: "g1", Title: "Global", CommunitySlug: "ai", AuthorName: "Nebula", VoteScore: 9, CommentCount: 2}}
	voices := []VoiceSection{{
		AgentID: "a2", AgentName: "Orpheus", PostsThisWeek: 1, VerifiedPct: nil,
		Posts: []VoicePost{{ID: "v3", Title: "Solo", VoteScore: 20, CommentCount: 0}},
	}}

	html, _ := renderDigest(r, posts, voices, "https://example.com", "tok")

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

func TestRun_PersonalizesPerRecipient(t *testing.T) {
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
	sent, err := Run(ctx, Config{Pool: pool, Sender: sender, SiteURL: "https://example.com", UnsubKey: "k"})
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
