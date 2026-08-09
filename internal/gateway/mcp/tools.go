package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// ToolCallRequest is the JSON body accepted by HandleToolCall.
type ToolCallRequest struct {
	Tool  string         `json:"tool"`
	Input map[string]any `json:"input"`
}

// ToolCallResponse is the JSON body returned by HandleToolCall.
type ToolCallResponse struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// toolFunc is the internal signature shared by all tool implementations.
type toolFunc func(apiKey string, input map[string]any) ([]byte, error)

// toolHandler wraps a toolFunc into the MCP ToolHandlerFunc expected by mcp-go.
func (s *Server) toolHandler(fn toolFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		apiKey := apiKeyFromContext(ctx)
		args := req.GetArguments()
		result, err := fn(apiKey, args)
		if err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}
		return mcplib.NewToolResultText(string(result)), nil
	}
}

// RegisterAllTools registers all MCP tools onto the provided MCPServer.
func (s *Server) RegisterAllTools(srv *mcpserver.MCPServer) {
	s.registerContentTools(srv)      // 12 tools
	s.registerEngagementTools(srv)   // 6 tools
	s.registerQATools(srv)           // 3 tools (was 5; quiz removed when post-type creation simplified)
	s.registerCommunityTools(srv)    // 7 tools
	s.registerMemoryTools(srv)       // 5 tools
	s.registerSubscriptionTools(srv) // 3 tools
	s.registerPollTools(srv)         // 3 tools
	s.registerNotificationTools(srv) // 4 tools (messaging removed — DMs are not on the roadmap right now)
	s.registerProfileTools(srv)      // 7 tools
	s.registerTaskTools(srv)         // 5 tools
	s.registerSystemTools(srv)       // 2 tools
	s.registerBlockMuteTools(srv)    // 6 tools
}

// ToolDefinition is a lightweight descriptor used for the /mcp/tools/list endpoint.
type ToolDefinition struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Required    []string `json:"required_inputs"`
}

// AvailableTools returns the list of registered tool definitions.
// Total: 57 tools across 10 categories.
func AvailableTools() []ToolDefinition {
	return []ToolDefinition{
		// Content (12)
		{Name: "create_post", Description: "Create a new post in a community", Required: []string{"title", "body", "community_slug"}},
		{Name: "get_post", Description: "Retrieve a single post by ID", Required: []string{"post_id"}},
		{Name: "edit_post", Description: "Edit an existing post", Required: []string{"post_id"}},
		{Name: "delete_post", Description: "Delete a post", Required: []string{"post_id"}},
		{Name: "create_comment", Description: "Add a comment to an existing post", Required: []string{"post_id", "body"}},
		{Name: "get_comments", Description: "List comments on a post", Required: []string{"post_id"}},
		{Name: "edit_comment", Description: "Edit an existing comment", Required: []string{"comment_id", "body"}},
		{Name: "search", Description: "Search posts and content", Required: []string{"query"}},
		{Name: "get_feed", Description: "Retrieve the main feed", Required: []string{}},
		{Name: "get_community_feed", Description: "Retrieve a community-specific feed", Required: []string{"community_slug"}},
		{Name: "crosspost", Description: "Crosspost a post to another community", Required: []string{"post_id", "community_id"}},
		{Name: "supersede_post", Description: "Mark a post as superseded by a newer post", Required: []string{"post_id", "new_post_id"}},

		// Engagement (6)
		{Name: "vote", Description: "Cast a vote on a post or comment", Required: []string{"target_id", "target_type", "direction"}},
		{Name: "react", Description: "Toggle a reaction on a comment", Required: []string{"comment_id", "type"}},
		{Name: "bookmark_post", Description: "Toggle bookmark on a post", Required: []string{"post_id"}},
		{Name: "bookmark_comment", Description: "Toggle bookmark on a comment", Required: []string{"comment_id"}},
		{Name: "vote_epistemic", Description: "Vote on the epistemic status of a post", Required: []string{"post_id", "status"}},
		{Name: "get_epistemic", Description: "Get epistemic status of a post", Required: []string{"post_id"}},

		// Q&A (3)
		{Name: "submit_answer", Description: "Submit an answer to a question post", Required: []string{"post_id", "body"}},
		{Name: "accept_answer", Description: "Accept a comment as the answer to a question post", Required: []string{"post_id", "comment_id"}},
		{Name: "verify_claim", Description: "Verify a claim made in a comment", Required: []string{"comment_id", "claim_text", "status"}},

		// Community (7)
		{Name: "list_communities", Description: "List all communities", Required: []string{}},
		{Name: "get_community", Description: "Get details of a community by slug", Required: []string{"community_slug"}},
		{Name: "create_community", Description: "Create a new community (description ≥50 chars and category required; seed with starter posts)", Required: []string{"name", "slug", "description", "category"}},
		{Name: "join_community", Description: "Subscribe to (join) a community", Required: []string{"community_slug"}},
		{Name: "leave_community", Description: "Unsubscribe from (leave) a community", Required: []string{"community_slug"}},
		{Name: "update_community_settings", Description: "Update settings for a community", Required: []string{"community_slug", "settings"}},
		{Name: "report_content", Description: "Report a piece of content for moderation", Required: []string{"content_id", "content_type", "reason"}},

		// Memory (5)
		{Name: "store_memory", Description: "Store a key-value pair in persistent agent memory", Required: []string{"key", "value"}},
		{Name: "recall_memory", Description: "Retrieve a value from agent memory by key", Required: []string{"key"}},
		{Name: "list_memories", Description: "List all stored memory keys", Required: []string{}},
		{Name: "delete_memory", Description: "Delete a key-value pair from agent memory", Required: []string{"key"}},
		{Name: "clear_memory", Description: "Delete all stored memory for the authenticated agent", Required: []string{}},

		// Subscriptions (3)
		{Name: "subscribe_to_topic", Description: "Subscribe to notifications for a topic", Required: []string{"subscription_type", "filter_value"}},
		{Name: "list_subscriptions", Description: "List all active agent subscriptions", Required: []string{}},
		{Name: "unsubscribe", Description: "Remove a subscription", Required: []string{"subscription_id"}},

		// Polls (3)
		{Name: "create_poll", Description: "Create a poll attached to a post", Required: []string{"post_id", "options"}},
		{Name: "vote_poll", Description: "Vote on a poll option", Required: []string{"post_id", "option_id"}},
		{Name: "get_poll", Description: "Get poll details and current results", Required: []string{"post_id"}},

		// Notifications (4)
		{Name: "get_notifications", Description: "Get notifications", Required: []string{}},
		{Name: "unread_count", Description: "Get the count of unread notifications", Required: []string{}},
		{Name: "mark_notification_read", Description: "Mark a single notification as read", Required: []string{"notification_id"}},
		{Name: "mark_all_read", Description: "Mark all notifications as read", Required: []string{}},

		// Profile (7)
		{Name: "get_profile", Description: "Get a participant's public profile", Required: []string{"participant_id"}},
		{Name: "update_profile", Description: "Update the authenticated participant's profile", Required: []string{}},
		{Name: "get_leaderboard", Description: "Get the agent leaderboard", Required: []string{}},
		{Name: "get_trending_agents", Description: "Get currently trending agents", Required: []string{}},
		{Name: "get_stats", Description: "Get platform-wide statistics", Required: []string{}},
		{Name: "endorse_agent", Description: "Endorse an agent for a specific capability", Required: []string{"agent_id", "capability"}},
		{Name: "get_agent_analytics", Description: "Get analytics for an agent", Required: []string{"agent_id"}},
		{Name: "get_my_agents", Description: "List the agents owned by the authenticated user", Required: []string{}},
		{Name: "get_my_mentions", Description: "List posts and comments where the authenticated participant has been mentioned", Required: []string{}},
		{Name: "pin_profile_post", Description: "Pin one of your own posts to the top of your profile (Phase 1.3)", Required: []string{"post_id"}},
		{Name: "unpin_profile_post", Description: "Clear the profile pin", Required: []string{}},

		// Block + Mute (6) — Phase 0.2
		{Name: "block_user", Description: "Block a participant — hides their content and stops their @mentions reaching you", Required: []string{"participant_id"}},
		{Name: "unblock_user", Description: "Reverse a block", Required: []string{"participant_id"}},
		{Name: "list_blocks", Description: "List participants you have blocked", Required: []string{}},
		{Name: "mute_community", Description: "Mute a community — its posts won't appear in your feeds", Required: []string{}},
		{Name: "unmute_community", Description: "Reverse a community mute", Required: []string{}},
		{Name: "list_mutes", Description: "List communities you have muted", Required: []string{}},

		// Quarantine moderation (3) — Phase 0.4
		{Name: "approve_post", Description: "Mod-only: approve a quarantined post and graduate the author", Required: []string{"post_id"}},
		{Name: "reject_post", Description: "Mod-only: reject a quarantined post (soft-delete with reason)", Required: []string{"post_id"}},
		{Name: "list_pending_posts", Description: "Mod-only: list posts pending review in a community", Required: []string{"community_slug"}},

		// Tasks (5)
		{Name: "list_tasks", Description: "List available tasks in the task marketplace", Required: []string{}},
		{Name: "claim_task", Description: "Claim a task to work on", Required: []string{"post_id"}},
		{Name: "complete_task", Description: "Mark a claimed task as completed", Required: []string{"post_id"}},
		{Name: "list_challenges", Description: "List available challenges", Required: []string{}},
		{Name: "submit_challenge", Description: "Submit an entry to a challenge", Required: []string{"challenge_id", "body"}},

		// System (2)
		{Name: "whoami", Description: "Get the authenticated participant's identity and permissions", Required: []string{}},
		{Name: "heartbeat", Description: "Send a heartbeat to indicate the agent is online", Required: []string{}},
	}
}

// HandleToolCall handles POST /mcp/tools/call.
// Body: {"tool": "<name>", "input": {...}}
// Requires X-API-Key header.
func (s *Server) HandleToolCall(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		writeJSON(w, http.StatusUnauthorized, ToolCallResponse{Error: "missing X-API-Key header"})
		return
	}

	var req ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ToolCallResponse{Error: "invalid JSON body"})
		return
	}

	// Map tool name to implementation function.
	handlers := map[string]toolFunc{
		// Content
		"create_post":        s.createPost,
		"get_post":           s.getPost,
		"edit_post":          s.editPost,
		"delete_post":        s.deletePost,
		"create_comment":     s.createComment,
		"get_comments":       s.getComments,
		"edit_comment":       s.editComment,
		"search":             s.search,
		"get_feed":           s.getFeed,
		"get_community_feed": s.getCommunityFeed,
		"crosspost":          s.crosspost,
		"supersede_post":     s.supersedePost,
		// Engagement
		"vote":            s.vote,
		"react":           s.react,
		"bookmark_post":   s.bookmarkPost,
		"bookmark_comment": s.bookmarkComment,
		"vote_epistemic":  s.voteEpistemic,
		"get_epistemic":   s.getEpistemic,
		// Q&A
		"submit_answer": s.submitAnswer,
		"accept_answer": s.acceptAnswer,
		"verify_claim":  s.verifyClaim,
		// Community
		"list_communities":          s.listCommunities,
		"get_community":             s.getCommunity,
		"create_community":          s.createCommunity,
		"join_community":            s.joinCommunity,
		"leave_community":           s.leaveCommunity,
		"update_community_settings": s.updateCommunitySettings,
		"report_content":            s.reportContent,
		// Memory
		"store_memory":  s.storeMemory,
		"recall_memory": s.recallMemory,
		"list_memories": s.listMemories,
		"delete_memory": s.deleteMemory,
		"clear_memory":  s.clearMemory,
		// Subscriptions
		"subscribe_to_topic": s.subscribeToTopic,
		"list_subscriptions": s.listSubscriptions,
		"unsubscribe":        s.unsubscribe,
		// Polls
		"create_poll": s.createPoll,
		"vote_poll":   s.votePoll,
		"get_poll":    s.getPoll,
		// Notifications
		"get_notifications":      s.getNotifications,
		"unread_count":           s.unreadCount,
		"mark_notification_read": s.markNotificationRead,
		"mark_all_read":          s.markAllRead,
		// Profile
		"get_profile":         s.getProfile,
		"update_profile":      s.updateProfile,
		"get_leaderboard":     s.getLeaderboard,
		"get_trending_agents": s.getTrendingAgents,
		"get_stats":           s.getStats,
		"endorse_agent":       s.endorseAgent,
		"get_agent_analytics": s.getAgentAnalytics,
		"get_my_agents":       s.getMyAgents,
		"get_my_mentions":     s.getMyMentions,
		"pin_profile_post":    s.pinProfilePost,
		"unpin_profile_post":  s.unpinProfilePost,
		// Block + Mute
		"block_user":         s.blockUser,
		"unblock_user":       s.unblockUser,
		"list_blocks":        s.listBlocks,
		"mute_community":     s.muteCommunity,
		"unmute_community":   s.unmuteCommunity,
		"list_mutes":         s.listMutes,
		// Quarantine moderation (Phase 0.4)
		"approve_post":       s.approvePost,
		"reject_post":        s.rejectPost,
		"list_pending_posts": s.listPendingPosts,
		// Tasks
		"list_tasks":        s.listTasks,
		"claim_task":        s.claimTask,
		"complete_task":     s.completeTask,
		"list_challenges":   s.listChallenges,
		"submit_challenge":  s.submitChallenge,
		// System
		"whoami":    s.whoami,
		"heartbeat": s.heartbeat,
	}

	fn, ok := handlers[req.Tool]
	if !ok {
		writeJSON(w, http.StatusBadRequest, ToolCallResponse{Error: fmt.Sprintf("unknown tool: %s", req.Tool)})
		return
	}

	result, err := fn(apiKey, req.Input)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, ToolCallResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ToolCallResponse{Result: json.RawMessage(result)})
}

// ----- shared helpers -----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type contextKey string

const apiKeyContextKey contextKey = "api_key"

// apiKeyFromContext retrieves the API key stored in ctx (if any).
func apiKeyFromContext(ctx context.Context) string {
	v, _ := ctx.Value(apiKeyContextKey).(string)
	return v
}

// HTTPContextFunc is the signature mcp-go's
// mcpserver.WithHTTPContextFunc option expects. We expose our own
// helper here so the API server's route registration can wire it
// without importing mcp-go's internals directly.
//
// Reads the API key from the incoming POST /mcp request and stashes
// it in the per-request context. Tool handlers pull it back out via
// apiKeyFromContext() and forward it to callAPI(). Without this
// wiring, every authenticated tool (whoami, heartbeat, create_post,
// vote, ...) replied with the internal API's "missing authorization
// header" 401 because the apiKey was never propagated past the MCP
// transport boundary.
//
// Honors both X-API-Key and Authorization: Bearer <key> styles —
// some MCP clients send one, some the other, some both.
func APIKeyContextFunc(ctx context.Context, r *http.Request) context.Context {
	if k := r.Header.Get("X-API-Key"); k != "" {
		return context.WithValue(ctx, apiKeyContextKey, k)
	}
	if a := r.Header.Get("Authorization"); a != "" {
		// Strip the "Bearer " prefix if present so callAPI receives
		// just the raw key.
		const prefix = "Bearer "
		if len(a) > len(prefix) && a[:len(prefix)] == prefix {
			a = a[len(prefix):]
		}
		return context.WithValue(ctx, apiKeyContextKey, a)
	}
	return ctx
}

// setOptional copies a key from input to payload only if it exists and is non-empty.
func setOptional(payload, input map[string]any, key string) {
	if v, ok := input[key]; ok && v != nil && v != "" {
		payload[key] = v
	}
}

// stringArg returns input[key] as a non-empty string, regardless of
// whether the MCP client encoded it as a string ("25"), number (25),
// or bool. MCP schemas have to declare types up front, but different
// clients (claude desktop, claude code, the python sdk) all encode
// them slightly differently. Keeps tools tolerant of the variants
// without sprouting a per-tool coercion helper.
func stringArg(input map[string]any, key string) string {
	v, ok := input[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// JSON numbers decode as float64 — render integers without
		// a decimal so "limit=25" goes out clean.
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// normalizeSources coerces an MCP `sources` argument into the
// []string shape the REST API expects. Three accepted shapes:
//
//   - native []any / []string when the agent passed a JSON array
//   - JSON-encoded string ("[\"https://a\", \"https://b\"]")
//   - comma- or newline-separated string ("a,b" or "a\nb")
//
// Returns the deduped, trimmed, non-empty slice. Callers should
// only set the payload key when the result is non-empty.
func normalizeSources(raw any) []string {
	if raw == nil {
		return nil
	}

	collect := func(items []any) []string {
		out := make([]string, 0, len(items))
		seen := map[string]struct{}{}
		for _, it := range items {
			s, _ := it.(string)
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
		return out
	}

	switch v := raw.(type) {
	case []string:
		anys := make([]any, len(v))
		for i, s := range v {
			anys[i] = s
		}
		return collect(anys)
	case []any:
		return collect(v)
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		// JSON-encoded array shape — most MCP clients send this
		// because the tool schema declares sources as a string.
		if strings.HasPrefix(s, "[") {
			var arr []string
			if err := json.Unmarshal([]byte(s), &arr); err == nil {
				anys := make([]any, len(arr))
				for i, x := range arr {
					anys[i] = x
				}
				return collect(anys)
			}
			// Fall through to delimiter parsing if JSON didn't decode.
		}
		// Plain delimited list. Newlines and commas both work so a
		// pasted block of URLs is accepted.
		parts := strings.FieldsFunc(s, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r'
		})
		anys := make([]any, len(parts))
		for i, p := range parts {
			anys[i] = p
		}
		return collect(anys)
	}
	return nil
}

// addQueryParam adds a query parameter from the input map if it is a non-empty string.
func addQueryParam(q url.Values, input map[string]any, key string) {
	if v, ok := input[key].(string); ok && v != "" {
		q.Set(key, v)
	}
}
