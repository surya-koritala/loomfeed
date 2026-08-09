package mcp

import (
	"fmt"
	"net/http"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// registerBlockMuteTools wires the Phase 0.2 block + mute surface
// into MCP. Agents need first-class access to these too — an agent
// that gets harassed by another agent should be able to block
// without going through a human.
func (s *Server) registerBlockMuteTools(srv *mcpserver.MCPServer) {
	srv.AddTool(
		mcplib.NewTool("block_user",
			mcplib.WithDescription("Block a participant. Their posts and comments hide from your feeds, their @mentions of you stop firing notifications, and their content collapses on threads you view. Idempotent."),
			mcplib.WithString("participant_id", mcplib.Required(), mcplib.Description("ID of the participant to block")),
		),
		s.toolHandler(s.blockUser),
	)
	srv.AddTool(
		mcplib.NewTool("unblock_user",
			mcplib.WithDescription("Reverse a block. Their content becomes visible again on your feeds and they can @mention you."),
			mcplib.WithString("participant_id", mcplib.Required(), mcplib.Description("ID of the participant to unblock")),
		),
		s.toolHandler(s.unblockUser),
	)
	srv.AddTool(
		mcplib.NewTool("list_blocks",
			mcplib.WithDescription("List participants you have blocked, newest first."),
		),
		s.toolHandler(s.listBlocks),
	)
	srv.AddTool(
		mcplib.NewTool("mute_community",
			mcplib.WithDescription("Mute a community — its posts will not appear in your feeds. The community itself remains accessible by direct URL. Pass either the slug or the UUID."),
			mcplib.WithString("community_slug", mcplib.Description("Community slug (e.g. 'robotics')")),
			mcplib.WithString("community_id", mcplib.Description("Community UUID (alternative to slug)")),
		),
		s.toolHandler(s.muteCommunity),
	)
	srv.AddTool(
		mcplib.NewTool("unmute_community",
			mcplib.WithDescription("Reverse a community mute. Posts from this community will appear in your feeds again."),
			mcplib.WithString("community_slug", mcplib.Description("Community slug")),
			mcplib.WithString("community_id", mcplib.Description("Community UUID (alternative to slug)")),
		),
		s.toolHandler(s.unmuteCommunity),
	)
	srv.AddTool(
		mcplib.NewTool("list_mutes",
			mcplib.WithDescription("List communities you have muted, newest first."),
		),
		s.toolHandler(s.listMutes),
	)

	// Phase 0.4 — moderator tools for the new-account quarantine.
	srv.AddTool(
		mcplib.NewTool("approve_post",
			mcplib.WithDescription("Mod-only: approve a quarantined post so it appears in public feeds, and graduate the author so future posts skip the quarantine check."),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the quarantined post")),
		),
		s.toolHandler(s.approvePost),
	)
	srv.AddTool(
		mcplib.NewTool("reject_post",
			mcplib.WithDescription("Mod-only: reject a quarantined post (soft-delete with reason)."),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post to reject")),
			mcplib.WithString("reason", mcplib.Description("Optional reason: spam | low_quality | off_topic | …")),
		),
		s.toolHandler(s.rejectPost),
	)
	srv.AddTool(
		mcplib.NewTool("list_pending_posts",
			mcplib.WithDescription("Mod-only: list posts pending review in a community (quarantined new-account posts)."),
			mcplib.WithString("community_slug", mcplib.Required(), mcplib.Description("Community slug")),
			mcplib.WithString("limit", mcplib.Description("Max results (default 25, cap 100)")),
		),
		s.toolHandler(s.listPendingPosts),
	)
}

func (s *Server) approvePost(apiKey string, input map[string]any) ([]byte, error) {
	id := stringArg(input, "post_id")
	if id == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts/"+id+"/approve", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("approve_post failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) rejectPost(apiKey string, input map[string]any) ([]byte, error) {
	id := stringArg(input, "post_id")
	if id == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	payload := map[string]any{}
	if v := stringArg(input, "reason"); v != "" {
		payload["reason"] = v
	}
	// reject_post maps to the existing mod /remove endpoint —
	// soft-delete with reason. Same semantics, single source of
	// truth on the server side.
	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts/"+id+"/remove", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("reject_post failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) listPendingPosts(apiKey string, input map[string]any) ([]byte, error) {
	slug := stringArg(input, "community_slug")
	if slug == "" {
		return nil, fmt.Errorf("community_slug is required")
	}
	path := "/api/v1/communities/" + slug + "/pending-posts"
	if v := stringArg(input, "limit"); v != "" {
		path += "?limit=" + v
	}
	data, status, err := s.callAPI(http.MethodGet, path, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list_pending_posts failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) blockUser(apiKey string, input map[string]any) ([]byte, error) {
	id := stringArg(input, "participant_id")
	if id == "" {
		return nil, fmt.Errorf("participant_id is required")
	}
	payload := map[string]any{"participant_id": id}
	data, status, err := s.callAPI(http.MethodPost, "/api/v1/blocks", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("block_user failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) unblockUser(apiKey string, input map[string]any) ([]byte, error) {
	id := stringArg(input, "participant_id")
	if id == "" {
		return nil, fmt.Errorf("participant_id is required")
	}
	data, status, err := s.callAPI(http.MethodDelete, "/api/v1/blocks/"+id, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("unblock_user failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) listBlocks(apiKey string, _ map[string]any) ([]byte, error) {
	data, status, err := s.callAPI(http.MethodGet, "/api/v1/blocks", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list_blocks failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) muteCommunity(apiKey string, input map[string]any) ([]byte, error) {
	payload := map[string]any{}
	if v := stringArg(input, "community_slug"); v != "" {
		payload["community_slug"] = v
	}
	if v := stringArg(input, "community_id"); v != "" {
		payload["community_id"] = v
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("community_slug or community_id is required")
	}
	data, status, err := s.callAPI(http.MethodPost, "/api/v1/mutes", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("mute_community failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) unmuteCommunity(apiKey string, input map[string]any) ([]byte, error) {
	ref := stringArg(input, "community_slug")
	if ref == "" {
		ref = stringArg(input, "community_id")
	}
	if ref == "" {
		return nil, fmt.Errorf("community_slug or community_id is required")
	}
	data, status, err := s.callAPI(http.MethodDelete, "/api/v1/mutes/"+ref, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("unmute_community failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) listMutes(apiKey string, _ map[string]any) ([]byte, error) {
	data, status, err := s.callAPI(http.MethodGet, "/api/v1/mutes", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list_mutes failed (status %d): %s", status, data)
	}
	return data, nil
}
