package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/testutil"
	"github.com/surya-koritala/loomfeed/internal/webhook"
)

type sourceEvent struct {
	typeName string
	payload  map[string]any
}

type sourceEventRecorder struct {
	mu     sync.Mutex
	events []sourceEvent
}

func (r *sourceEventRecorder) Dispatch(eventType string, payload map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, sourceEvent{typeName: eventType, payload: payload})
}

func (r *sourceEventRecorder) only(t *testing.T) sourceEvent {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		if len(r.events) == 1 {
			event := r.events[0]
			r.mu.Unlock()
			return event
		}
		if len(r.events) > 1 {
			events := append([]sourceEvent(nil), r.events...)
			r.mu.Unlock()
			t.Fatalf("recorded events=%#v, want one", events)
		}
		r.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for webhook source event")
	return sourceEvent{}
}

func (r *sourceEventRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func TestPostCreatedWebhookFiresAfterPersistedCreate(t *testing.T) {
	handler, participants, communities, cfg := setupPostTest(t)
	author, token := registerTestUser(t, participants, cfg, "webhook-post@example.com", "Webhook Poster")
	community := createTestCommunity(t, communities, author.ID, "webhook-post")
	recorder := &sourceEventRecorder{}
	handler.WithWebhook(recorder)

	request := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts", token, models.CreatePostRequest{
		CommunityID: community.ID,
		Title:       "Truthful webhook post",
		Body:        "The dispatcher must observe only committed posts.",
		Tags:        []string{"webhooks"},
	})
	response := httptest.NewRecorder()
	middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create)).ServeHTTP(response, request)
	testutil.AssertStatus(t, response, http.StatusCreated)

	var created models.CreatePostResponse
	testutil.DecodeResponse(t, response, &created)
	event := recorder.only(t)
	if event.typeName != webhook.EventPostCreated {
		t.Fatalf("event type=%q", event.typeName)
	}
	if event.payload["post_id"] != created.ID || event.payload["community_id"] != community.ID || event.payload["author_id"] != author.ID {
		t.Fatalf("post.created payload=%#v", event.payload)
	}
	getMux := http.NewServeMux()
	getMux.HandleFunc("GET /api/v1/posts/{id}", handler.Get)
	getResponse := httptest.NewRecorder()
	getMux.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+created.ID, nil))
	testutil.AssertStatus(t, getResponse, http.StatusOK)
}

func TestQuarantinedPostWebhookWaitsForPublicationApproval(t *testing.T) {
	handler, participants, communities, cfg := setupPostTest(t)
	author, token := registerTestUser(t, participants, cfg, "quarantined-webhook@example.com", "Quarantined Webhook Author")
	community := createTestCommunity(t, communities, author.ID, "quarantined-webhook")
	pool := database.TestPool(t)
	if _, err := pool.Exec(context.Background(), `UPDATE participants SET trust_score = 0 WHERE id = $1`, author.ID); err != nil {
		t.Fatalf("set low trust for quarantine boundary: %v", err)
	}
	recorder := &sourceEventRecorder{}
	handler.WithParticipants(participants)
	handler.WithWebhook(recorder)

	createRequest := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts", token, models.CreatePostRequest{
		CommunityID: community.ID,
		Title:       "Private until approved",
		Body:        "This content must not leave the moderation boundary early.",
	})
	createResponse := httptest.NewRecorder()
	middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create)).ServeHTTP(createResponse, createRequest)
	testutil.AssertStatus(t, createResponse, http.StatusCreated)
	var created models.CreatePostResponse
	testutil.DecodeResponse(t, createResponse, &created)
	if !created.Quarantined {
		t.Fatal("fresh low-trust human post should be quarantined")
	}
	if got := recorder.count(); got != 0 {
		t.Fatalf("quarantined post disclosed through %d webhook events", got)
	}

	posts := repository.NewPostRepo(pool)
	moderationHandler := handlers.NewModActionHandler(
		repository.NewModActionRepo(pool),
		repository.NewModerationRepo(pool),
		communities,
		repository.NewReportRepo(pool),
	)
	moderationHandler.WithPostsAndAccount(posts, repository.NewAccountRepo(pool))
	moderationHandler.WithWebhook(recorder)
	approveMux := http.NewServeMux()
	approveMux.Handle("POST /api/v1/posts/{id}/approve", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(moderationHandler.ApprovePost)))
	approveResponse := httptest.NewRecorder()
	approveMux.ServeHTTP(approveResponse, testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+created.ID+"/approve", token, map[string]any{}))
	testutil.AssertStatus(t, approveResponse, http.StatusOK)

	event := recorder.only(t)
	if event.typeName != webhook.EventPostCreated || event.payload["post_id"] != created.ID {
		t.Fatalf("publication event=%#v", event)
	}
	published, err := posts.GetByID(context.Background(), created.ID)
	if err != nil || published.Quarantined {
		t.Fatalf("published post=%+v err=%v", published, err)
	}

	// Approval is idempotent: an already-public post must not be announced twice.
	again := httptest.NewRecorder()
	approveMux.ServeHTTP(again, testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+created.ID+"/approve", token, map[string]any{}))
	testutil.AssertStatus(t, again, http.StatusOK)
	if got := recorder.count(); got != 1 {
		t.Fatalf("re-approval emitted %d publication events, want one", got)
	}
}

func TestConcurrentQuarantinedPostApprovalEmitsOnce(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"moderation_actions", "reports", "posts", "communities", "human_users", "participants",
	)
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "concurrent-approval-test", Expiry: time.Hour}}
	moderator, token := registerTestUser(t, participants, cfg, "concurrent-approval@example.com", "Concurrent Approver")
	community := createTestCommunity(t, communities, moderator.ID, "concurrent-approval")
	post, err := posts.Create(context.Background(), &models.Post{
		CommunityID: community.ID, AuthorID: moderator.ID, AuthorType: models.ParticipantHuman,
		Title: "Concurrent publication", Body: "Only one event may escape.", PostType: models.PostTypeText,
		Quarantined: true,
	})
	if err != nil {
		t.Fatalf("create quarantined post: %v", err)
	}
	recorder := &sourceEventRecorder{}
	handler := handlers.NewModActionHandler(
		repository.NewModActionRepo(pool), repository.NewModerationRepo(pool), communities, repository.NewReportRepo(pool),
	)
	handler.WithPostsAndAccount(posts, repository.NewAccountRepo(pool))
	handler.WithWebhook(recorder)
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/posts/{id}/approve", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.ApprovePost)))

	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/approve", token, map[string]any{}))
			results <- response
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for result := range results {
		testutil.AssertStatus(t, result, http.StatusOK)
	}
	if got := recorder.count(); got != 1 {
		t.Fatalf("concurrent approval emitted %d publication events, want one", got)
	}
}

func TestAnswerAcceptedWebhookFiresAfterAtomicQuestionUpdate(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"reputation_events", "comments", "posts", "communities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	comments := repository.NewCommentRepo(pool)

	cfg := &config.Config{JWT: config.JWTConfig{Secret: "answer-webhook-test", Expiry: time.Hour}}
	author, authorToken := registerTestUser(t, participants, cfg, "question-author@example.com", "Question Author")
	answerer, _ := registerTestUser(t, participants, cfg, "question-answerer@example.com", "Question Answerer")
	community := createTestCommunity(t, communities, author.ID, "answer-webhook")
	question, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID,
		AuthorID:    author.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "What makes a webhook truthful?",
		Body:        "Explain the transaction boundary.",
		PostType:    models.PostTypeQuestion,
	})
	if err != nil {
		t.Fatalf("create question: %v", err)
	}
	answer, err := comments.Create(ctx, &models.Comment{
		PostID:     question.ID,
		AuthorID:   answerer.ID,
		AuthorType: models.ParticipantHuman,
		Body:       "Emit only after the accepted-answer update commits.",
	})
	if err != nil {
		t.Fatalf("create answer: %v", err)
	}

	handler := handlers.NewReactionHandler(repository.NewReactionRepo(pool), posts, comments, repository.NewReputationRepo(pool), cfg)
	recorder := &sourceEventRecorder{}
	handler.WithWebhook(recorder)
	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/posts/{id}/accept-answer", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.AcceptAnswer)))
	request := testutil.JSONRequestWithAuth(t, http.MethodPut, "/api/v1/posts/"+question.ID+"/accept-answer", authorToken, map[string]string{
		"comment_id": answer.ID,
	})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	testutil.AssertStatus(t, response, http.StatusOK)

	event := recorder.only(t)
	if event.typeName != webhook.EventAnswerAccepted {
		t.Fatalf("event type=%q", event.typeName)
	}
	if event.payload["post_id"] != question.ID || event.payload["comment_id"] != answer.ID || event.payload["answer_author_id"] != answerer.ID || event.payload["accepted_by"] != author.ID {
		t.Fatalf("answer.accepted payload=%#v", event.payload)
	}
	var acceptedID *string
	var status *string
	if err := pool.QueryRow(ctx, `SELECT accepted_answer_id, question_status FROM posts WHERE id = $1`, question.ID).Scan(&acceptedID, &status); err != nil {
		t.Fatalf("read accepted question: %v", err)
	}
	if acceptedID == nil || *acceptedID != answer.ID || status == nil || *status != string(models.QuestionStatusAnswered) {
		t.Fatalf("question state accepted=%v status=%v", acceptedID, status)
	}
}

func TestConcurrentAnswerDeletionSuppressesAcceptedEvent(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"reputation_events", "comments", "posts", "communities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	comments := repository.NewCommentRepo(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "answer-delete-race-test", Expiry: time.Hour}}
	author, token := registerTestUser(t, participants, cfg, "answer-delete-author@example.com", "Answer Delete Author")
	answerer, _ := registerTestUser(t, participants, cfg, "answer-delete-answerer@example.com", "Answer Delete Answerer")
	community := createTestCommunity(t, communities, author.ID, "answer-delete-race")
	question, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID, AuthorID: author.ID, AuthorType: models.ParticipantHuman,
		Title: "Concurrent answer deletion", Body: "Deletion must win cleanly.", PostType: models.PostTypeQuestion,
	})
	if err != nil {
		t.Fatalf("create question: %v", err)
	}
	answer, err := comments.Create(ctx, &models.Comment{
		PostID: question.ID, AuthorID: answerer.ID, AuthorType: models.ParticipantHuman, Body: "Delete me concurrently.",
	})
	if err != nil {
		t.Fatalf("create answer: %v", err)
	}

	deleteTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin concurrent delete: %v", err)
	}
	defer func() { _ = deleteTx.Rollback(ctx) }()
	if _, err := deleteTx.Exec(ctx, `UPDATE comments SET deleted_at = NOW() WHERE id = $1`, answer.ID); err != nil {
		t.Fatalf("stage concurrent answer deletion: %v", err)
	}

	recorder := &sourceEventRecorder{}
	handler := handlers.NewReactionHandler(repository.NewReactionRepo(pool), posts, comments, repository.NewReputationRepo(pool), cfg)
	handler.WithWebhook(recorder)
	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/posts/{id}/accept-answer", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.AcceptAnswer)))
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		result := httptest.NewRecorder()
		mux.ServeHTTP(result, testutil.JSONRequestWithAuth(t, http.MethodPut, "/api/v1/posts/"+question.ID+"/accept-answer", token,
			map[string]string{"comment_id": answer.ID}))
		response <- result
	}()
	select {
	case result := <-response:
		t.Fatalf("acceptance did not wait for concurrent deletion: status=%d body=%s", result.Code, result.Body.String())
	case <-time.After(100 * time.Millisecond):
	}
	if err := deleteTx.Commit(ctx); err != nil {
		t.Fatalf("commit concurrent answer deletion: %v", err)
	}
	result := <-response
	if result.Code != http.StatusBadRequest {
		t.Fatalf("accept deleted answer status=%d body=%s", result.Code, result.Body.String())
	}
	if got := recorder.count(); got != 0 {
		t.Fatalf("concurrently deleted answer disclosed through %d webhook events", got)
	}
}

func TestQuarantinedQuestionDoesNotDiscloseAcceptedAnswer(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"reputation_events", "comments", "posts", "communities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	comments := repository.NewCommentRepo(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "hidden-answer-webhook-test", Expiry: time.Hour}}
	author, token := registerTestUser(t, participants, cfg, "hidden-question@example.com", "Hidden Question Author")
	answerer, _ := registerTestUser(t, participants, cfg, "hidden-answer@example.com", "Hidden Answer Author")
	community := createTestCommunity(t, communities, author.ID, "hidden-answer-webhook")
	question, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID, AuthorID: author.ID, AuthorType: models.ParticipantHuman,
		Title: "Hidden question", Body: "Do not disclose this accepted answer.",
		PostType: models.PostTypeQuestion, Quarantined: true,
	})
	if err != nil {
		t.Fatalf("create hidden question: %v", err)
	}
	answer, err := comments.Create(ctx, &models.Comment{
		PostID: question.ID, AuthorID: answerer.ID, AuthorType: models.ParticipantHuman, Body: "Hidden answer",
	})
	if err != nil {
		t.Fatalf("create hidden answer: %v", err)
	}

	recorder := &sourceEventRecorder{}
	handler := handlers.NewReactionHandler(repository.NewReactionRepo(pool), posts, comments, repository.NewReputationRepo(pool), cfg)
	handler.WithWebhook(recorder)
	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/posts/{id}/accept-answer", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.AcceptAnswer)))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, testutil.JSONRequestWithAuth(t, http.MethodPut, "/api/v1/posts/"+question.ID+"/accept-answer", token, map[string]string{"comment_id": answer.ID}))
	testutil.AssertStatus(t, response, http.StatusOK)
	if got := recorder.count(); got != 0 {
		t.Fatalf("quarantined answer disclosed through %d webhook events", got)
	}
}

func TestCommentCreatedAndMentionWebhooksFireAfterPersistedCreate(t *testing.T) {
	handler, participants, communities, posts, cfg := setupCommentTest(t)
	commenter, token := registerTestUser(t, participants, cfg, "webhook-commenter@example.com", "Webhook Commenter")
	mentioned, _ := registerTestUser(t, participants, cfg, "webhook-mentioned@example.com", "WebhookMentioned")
	community := createTestCommunity(t, communities, commenter.ID, "webhook-comment")
	post := createTestPost(t, posts, community.ID, commenter.ID)
	recorder := &sourceEventRecorder{}
	handler.WithParticipants(participants)
	handler.WithMentions(repository.NewMentionRepo(database.TestPool(t)))
	handler.WithWebhook(recorder, nil)

	request := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/comments", token, models.CreateCommentRequest{
		Body: strings.Repeat("界", 205) + " for @WebhookMentioned",
	})
	response := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/posts/{id}/comments", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create)))
	mux.ServeHTTP(response, request)
	testutil.AssertStatus(t, response, http.StatusCreated)
	var created models.CommentWithAuthor
	testutil.DecodeResponse(t, response, &created)

	deadline := time.Now().Add(2 * time.Second)
	var events []sourceEvent
	for time.Now().Before(deadline) {
		recorder.mu.Lock()
		events = append([]sourceEvent(nil), recorder.events...)
		recorder.mu.Unlock()
		if len(events) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(events) != 2 {
		t.Fatalf("recorded events=%#v, want comment.created and mention", events)
	}
	byType := map[string]map[string]any{}
	for _, event := range events {
		byType[event.typeName] = event.payload
	}
	if payload := byType[webhook.EventCommentCreated]; payload["comment_id"] != created.ID || payload["post_id"] != post.ID || payload["author_id"] != commenter.ID {
		t.Fatalf("comment.created payload=%#v", payload)
	}
	excerpt, ok := byType[webhook.EventCommentCreated]["body_excerpt"].(string)
	if !ok || len([]rune(excerpt)) != 200 || !strings.HasSuffix(excerpt, "...") {
		t.Fatalf("comment.created body_excerpt=%q (%d runes)", excerpt, len([]rune(excerpt)))
	}
	if payload := byType[webhook.EventMention]; payload["comment_id"] != created.ID || payload["post_id"] != post.ID || payload["mentioned_by"] != commenter.ID || payload["mentioned_id"] != mentioned.ID {
		t.Fatalf("mention payload=%#v", payload)
	}
	if _, err := repository.NewCommentRepo(database.TestPool(t)).GetByID(context.Background(), created.ID); err != nil {
		t.Fatalf("comment was not persisted before dispatch: %v", err)
	}
}

func TestVoteReceivedWebhookFiresAfterVoteTransaction(t *testing.T) {
	handler, participants, communities, posts, cfg := setupVoteTest(t)
	author, _ := registerTestUser(t, participants, cfg, "webhook-vote-author@example.com", "Webhook Vote Author")
	voter, voterToken := registerTestUser(t, participants, cfg, "webhook-voter@example.com", "Webhook Voter")
	community := createTestCommunity(t, communities, author.ID, "webhook-vote")
	post := createTestPost(t, posts, community.ID, author.ID)
	recorder := &sourceEventRecorder{}
	handler.WithWebhook(recorder, nil)

	request := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/votes", voterToken, models.VoteRequest{
		TargetID: post.ID, TargetType: "post", Direction: "up",
	})
	response := httptest.NewRecorder()
	middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Cast)).ServeHTTP(response, request)
	testutil.AssertStatus(t, response, http.StatusOK)

	event := recorder.only(t)
	if event.typeName != webhook.EventVoteReceived {
		t.Fatalf("event type=%q", event.typeName)
	}
	if event.payload["target_id"] != post.ID || event.payload["target_type"] != "post" || event.payload["voter_id"] != voter.ID || event.payload["direction"] != "up" {
		t.Fatalf("vote.received payload=%#v", event.payload)
	}
	var direction string
	if err := database.TestPool(t).QueryRow(context.Background(), `SELECT direction FROM votes WHERE target_id = $1 AND voter_id = $2`, post.ID, voter.ID).Scan(&direction); err != nil || direction != "up" {
		t.Fatalf("vote was not persisted before dispatch: direction=%q err=%v", direction, err)
	}

	// Repeating the same upvote toggles it off and must not claim that another
	// upvote was received.
	toggleResponse := httptest.NewRecorder()
	middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Cast)).ServeHTTP(toggleResponse,
		testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/votes", voterToken, models.VoteRequest{
			TargetID: post.ID, TargetType: "post", Direction: "up",
		}))
	testutil.AssertStatus(t, toggleResponse, http.StatusOK)
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.events) != 1 {
		t.Fatalf("toggle-off dispatched webhook events=%#v", recorder.events)
	}
	var remainingVotes int
	if err := database.TestPool(t).QueryRow(context.Background(), `SELECT COUNT(*) FROM votes WHERE target_id = $1 AND voter_id = $2`, post.ID, voter.ID).Scan(&remainingVotes); err != nil || remainingVotes != 0 {
		t.Fatalf("toggle-off vote count=%d err=%v", remainingVotes, err)
	}
}

func TestQuarantinedContentDoesNotDiscloseCommentMentionOrVote(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"notifications", "mentions", "reputation_events", "votes", "comments", "posts", "communities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	comments := repository.NewCommentRepo(pool)
	provenances := repository.NewProvenanceRepo(pool)
	notifications := repository.NewNotificationRepo(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "hidden-content-webhook-test", Expiry: time.Hour}}
	author, _ := registerTestUser(t, participants, cfg, "hidden-content-author@example.com", "Hidden Content Author")
	commenter, commenterToken := registerTestUser(t, participants, cfg, "hidden-content-commenter@example.com", "Hidden Content Commenter")
	mentioned, _ := registerTestUser(t, participants, cfg, "hidden-content-mentioned@example.com", "HiddenMentioned")
	voter, voterToken := registerTestUser(t, participants, cfg, "hidden-content-voter@example.com", "Hidden Content Voter")
	community := createTestCommunity(t, communities, author.ID, "hidden-content-webhook")
	post, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID, AuthorID: author.ID, AuthorType: models.ParticipantHuman,
		Title: "Hidden content", Body: "Private moderation content.", PostType: models.PostTypeText, Quarantined: true,
	})
	if err != nil {
		t.Fatalf("create hidden content: %v", err)
	}
	recorder := &sourceEventRecorder{}
	commentHandler := handlers.NewCommentHandler(comments, provenances, notifications, cfg)
	commentHandler.WithPosts(posts)
	commentHandler.WithParticipants(participants)
	commentHandler.WithMentions(repository.NewMentionRepo(pool))
	commentHandler.WithWebhook(recorder, nil)
	commentMux := http.NewServeMux()
	commentMux.Handle("POST /api/v1/posts/{id}/comments", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(commentHandler.Create)))
	commentResponse := httptest.NewRecorder()
	commentMux.ServeHTTP(commentResponse, testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/comments", commenterToken,
		models.CreateCommentRequest{Body: "Hidden mention for @HiddenMentioned"}))
	testutil.AssertStatus(t, commentResponse, http.StatusCreated)
	var createdComment models.CommentWithAuthor
	testutil.DecodeResponse(t, commentResponse, &createdComment)
	if createdComment.ID == "" || mentioned.ID == "" || commenter.ID == "" {
		t.Fatal("hidden comment fixtures were not persisted")
	}

	voteHandler := handlers.NewVoteHandler(repository.NewVoteRepo(pool), posts, comments, repository.NewReputationRepo(pool), cfg)
	voteHandler.WithWebhook(recorder, nil)
	voteResponse := httptest.NewRecorder()
	middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(voteHandler.Cast)).ServeHTTP(voteResponse,
		testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/votes", voterToken,
			models.VoteRequest{TargetID: post.ID, TargetType: "post", Direction: "up"}))
	testutil.AssertStatus(t, voteResponse, http.StatusOK)

	// Mention resolution is asynchronous; wait long enough to prove it cannot
	// enqueue a global webhook after the request returns.
	time.Sleep(150 * time.Millisecond)
	if got := recorder.count(); got != 0 {
		t.Fatalf("quarantined comment/mention/vote disclosed through %d webhook events", got)
	}
	var storedVote int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM votes WHERE target_id = $1 AND voter_id = $2`, post.ID, voter.ID).Scan(&storedVote); err != nil || storedVote != 1 {
		t.Fatalf("hidden vote persistence count=%d err=%v", storedVote, err)
	}
}

func TestDeletedContentDoesNotDiscloseVote(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"reputation_events", "votes", "comments", "posts", "communities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	comments := repository.NewCommentRepo(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "deleted-vote-webhook-test", Expiry: time.Hour}}
	author, _ := registerTestUser(t, participants, cfg, "deleted-vote-author@example.com", "Deleted Vote Author")
	_, voterToken := registerTestUser(t, participants, cfg, "deleted-vote-voter@example.com", "Deleted Vote Voter")
	community := createTestCommunity(t, communities, author.ID, "deleted-vote-webhook")
	post := createTestPost(t, posts, community.ID, author.ID)
	comment, err := comments.Create(ctx, &models.Comment{
		PostID: post.ID, AuthorID: author.ID, AuthorType: models.ParticipantHuman, Body: "Deleted comment",
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE comments SET deleted_at = NOW() WHERE id = $1`, comment.ID); err != nil {
		t.Fatalf("soft-delete comment: %v", err)
	}

	recorder := &sourceEventRecorder{}
	handler := handlers.NewVoteHandler(repository.NewVoteRepo(pool), posts, comments, repository.NewReputationRepo(pool), cfg)
	handler.WithWebhook(recorder, nil)
	cast := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Cast))
	commentVote := httptest.NewRecorder()
	cast.ServeHTTP(commentVote, testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/votes", voterToken,
		models.VoteRequest{TargetID: comment.ID, TargetType: "comment", Direction: "up"}))
	testutil.AssertStatus(t, commentVote, http.StatusOK)

	if _, err := pool.Exec(ctx, `UPDATE posts SET deleted_at = NOW() WHERE id = $1`, post.ID); err != nil {
		t.Fatalf("soft-delete post: %v", err)
	}
	postVote := httptest.NewRecorder()
	cast.ServeHTTP(postVote, testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/votes", voterToken,
		models.VoteRequest{TargetID: post.ID, TargetType: "post", Direction: "up"}))
	testutil.AssertStatus(t, postVote, http.StatusOK)

	if got := recorder.count(); got != 0 {
		t.Fatalf("deleted comment/post disclosed through %d vote webhook events", got)
	}
}
