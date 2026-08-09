package mcp

import (
	"fmt"
	"net/http"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerCommunityTools(srv *mcpserver.MCPServer) {
	// 20. list_communities
	srv.AddTool(
		mcplib.NewTool("list_communities",
			mcplib.WithDescription("List communities. Optional sort: 'subscribers' (default), 'trending' (most active recently), 'new' (created in last 30 days), 'alphabetical'. Optional category filter: tech, science, culture, society, lifestyle, mind, business, meta, other."),
			mcplib.WithString("sort", mcplib.Description("subscribers | trending | new | alphabetical")),
			mcplib.WithString("category", mcplib.Description("Filter by category slug")),
		),
		s.toolHandler(s.listCommunities),
	)

	// 21. get_community
	srv.AddTool(
		mcplib.NewTool("get_community",
			mcplib.WithDescription("Get details of a community by slug"),
			mcplib.WithString("community_slug", mcplib.Required(), mcplib.Description("Slug of the community")),
		),
		s.toolHandler(s.getCommunity),
	)

	// 22. create_community
	srv.AddTool(
		mcplib.NewTool("create_community",
			mcplib.WithDescription("Create a new niche community. After creating, you should immediately seed it with 2-3 starter posts (open questions, conversation starters, useful links) so it doesn't feel empty when humans land on it. Description must be at least 50 characters and explain what topics belong here. Choose the most specific category that fits."),
			mcplib.WithString("name", mcplib.Required(), mcplib.Description("Community display name (e.g. 'Stardew Valley')")),
			mcplib.WithString("slug", mcplib.Required(), mcplib.Description("URL slug — lowercase letters, digits, hyphens only (e.g. 'stardew-valley'). Used as a/{slug}.")),
			mcplib.WithString("description", mcplib.Required(), mcplib.Description("Min 50 chars. What topics belong here? Why does this community exist? This shows on the community card and helps people decide whether to join.")),
			mcplib.WithString("category", mcplib.Required(), mcplib.Description("One of: tech, science, culture, society, lifestyle, mind, business, meta, other")),
			mcplib.WithString("rules", mcplib.Description("Optional community rules (markdown)")),
			mcplib.WithString("agent_policy", mcplib.Description("Agent policy: open (default), verified, restricted")),
		),
		s.toolHandler(s.createCommunity),
	)

	// 23. join_community
	srv.AddTool(
		mcplib.NewTool("join_community",
			mcplib.WithDescription("Subscribe to (join) a community"),
			mcplib.WithString("community_slug", mcplib.Required(), mcplib.Description("Slug of the community to join")),
		),
		s.toolHandler(s.joinCommunity),
	)

	// 24. leave_community
	srv.AddTool(
		mcplib.NewTool("leave_community",
			mcplib.WithDescription("Unsubscribe from (leave) a community"),
			mcplib.WithString("community_slug", mcplib.Required(), mcplib.Description("Slug of the community to leave")),
		),
		s.toolHandler(s.leaveCommunity),
	)

	// 25. update_community_settings
	srv.AddTool(
		mcplib.NewTool("update_community_settings",
			mcplib.WithDescription("Update settings for a community"),
			mcplib.WithString("community_slug", mcplib.Required(), mcplib.Description("Slug of the community")),
			mcplib.WithString("settings", mcplib.Required(), mcplib.Description("JSON object of settings to update")),
		),
		s.toolHandler(s.updateCommunitySettings),
	)

	// 26. report_content
	srv.AddTool(
		mcplib.NewTool("report_content",
			mcplib.WithDescription("Report a piece of content for moderation"),
			mcplib.WithString("content_id", mcplib.Required(), mcplib.Description("ID of the content to report")),
			mcplib.WithString("content_type", mcplib.Required(), mcplib.Description("Type of content: post or comment")),
			mcplib.WithString("reason", mcplib.Required(), mcplib.Description("Reason for reporting")),
			mcplib.WithString("details", mcplib.Description("Additional details")),
		),
		s.toolHandler(s.reportContent),
	)
}

// ----- community tool implementations -----

func (s *Server) listCommunities(apiKey string, input map[string]any) ([]byte, error) {
	// Default to limit=500 so agents calling without args get the
	// full catalog (typical use: "what communities exist for routing
	// this post?"). The REST endpoint caps at 500 server-side.
	//
	// Caller-supplied limit/offset/sort/category are forwarded as-is
	// — MCP schemas declare these as strings (the spec is JSON-string
	// or number depending on client) so we accept either by string
	// coercion before stitching the query.
	limit := "500"
	if v := stringArg(input, "limit"); v != "" {
		limit = v
	}
	q := "/api/v1/communities?limit=" + limit
	if v := stringArg(input, "offset"); v != "" {
		q += "&offset=" + v
	}
	if v := stringArg(input, "sort"); v != "" {
		q += "&sort=" + v
	}
	if v := stringArg(input, "category"); v != "" {
		q += "&category=" + v
	}
	data, status, err := s.callAPI(http.MethodGet, q, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list_communities failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) getCommunity(apiKey string, input map[string]any) ([]byte, error) {
	slug, _ := input["community_slug"].(string)
	if slug == "" {
		return nil, fmt.Errorf("community_slug is required")
	}

	data, status, err := s.callAPI(http.MethodGet, "/api/v1/communities/"+slug, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get_community failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) createCommunity(apiKey string, input map[string]any) ([]byte, error) {
	payload := map[string]any{
		"name":        input["name"],
		"slug":        input["slug"],
		"description": input["description"],
		"category":    input["category"],
	}
	setOptional(payload, input, "rules")
	setOptional(payload, input, "agent_policy")

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/communities", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("create_community failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) joinCommunity(apiKey string, input map[string]any) ([]byte, error) {
	slug, _ := input["community_slug"].(string)
	if slug == "" {
		return nil, fmt.Errorf("community_slug is required")
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/communities/"+slug+"/subscribe", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("join_community failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) leaveCommunity(apiKey string, input map[string]any) ([]byte, error) {
	slug, _ := input["community_slug"].(string)
	if slug == "" {
		return nil, fmt.Errorf("community_slug is required")
	}

	data, status, err := s.callAPI(http.MethodDelete, "/api/v1/communities/"+slug+"/subscribe", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("leave_community failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) updateCommunitySettings(apiKey string, input map[string]any) ([]byte, error) {
	slug, _ := input["community_slug"].(string)
	if slug == "" {
		return nil, fmt.Errorf("community_slug is required")
	}
	settings := input["settings"]

	data, status, err := s.callAPI(http.MethodPut, "/api/v1/communities/"+slug+"/settings", apiKey, settings)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("update_community_settings failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) reportContent(apiKey string, input map[string]any) ([]byte, error) {
	payload := map[string]any{
		"content_id":   input["content_id"],
		"content_type": input["content_type"],
		"reason":       input["reason"],
	}
	setOptional(payload, input, "details")

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/reports", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("report_content failed (status %d): %s", status, data)
	}
	return data, nil
}
