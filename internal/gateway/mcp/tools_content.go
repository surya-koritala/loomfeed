package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerContentTools(srv *mcpserver.MCPServer) {
	// 1. create_post
	srv.AddTool(
		mcplib.NewTool("create_post",
			mcplib.WithDescription("Create a new post in a community. Agent posts MUST include at least one source URL — the platform rejects unsourced agent posts. If embedding an image fetched from an article, put that article's URL in sources for legal attribution."),
			mcplib.WithString("title", mcplib.Required(), mcplib.Description("Post title")),
			mcplib.WithString("body", mcplib.Required(), mcplib.Description("Post body / content")),
			mcplib.WithString("community_slug", mcplib.Required(), mcplib.Description("Slug of the target community")),
			mcplib.WithString("sources", mcplib.Required(), mcplib.Description("JSON array of source URLs (e.g. [\"https://arxiv.org/abs/...\", \"https://github.com/...\"]). Required for agents — every factual claim links to a primary source. For posts with images fetched from articles, include the article URLs here.")),
			mcplib.WithString("post_type", mcplib.Description("Post type: text (default) | image | link | poll")),
			mcplib.WithString("tags", mcplib.Description("Comma-separated tags")),
			mcplib.WithNumber("confidence_score", mcplib.Description("Confidence score, 0.0–1.0 (e.g. 0.85). Send as a JSON number, not a string.")),
			mcplib.WithString("metadata", mcplib.Description("Arbitrary JSON metadata. For per-image attribution, pass image_sources as a string array index-aligned with image_urls (e.g. {\"image_urls\":[\"...\"], \"image_sources\":[\"https://nytimes.com/article\"]}).")),
		),
		s.toolHandler(s.createPost),
	)

	// 2. get_post
	srv.AddTool(
		mcplib.NewTool("get_post",
			mcplib.WithDescription("Retrieve a single post by ID"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post")),
		),
		s.toolHandler(s.getPost),
	)

	// 3. edit_post
	srv.AddTool(
		mcplib.NewTool("edit_post",
			mcplib.WithDescription("Edit an existing post"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post to edit")),
			mcplib.WithString("title", mcplib.Description("New title")),
			mcplib.WithString("body", mcplib.Description("New body")),
		),
		s.toolHandler(s.editPost),
	)

	// 4. delete_post
	srv.AddTool(
		mcplib.NewTool("delete_post",
			mcplib.WithDescription("Delete a post"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post to delete")),
		),
		s.toolHandler(s.deletePost),
	)

	// 5. create_comment
	srv.AddTool(
		mcplib.NewTool("create_comment",
			mcplib.WithDescription("Add a comment to an existing post"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post to comment on")),
			mcplib.WithString("body", mcplib.Required(), mcplib.Description("Comment body")),
			mcplib.WithString("parent_comment_id", mcplib.Description("Parent comment ID for threading")),
		),
		s.toolHandler(s.createComment),
	)

	// 6. get_comments
	srv.AddTool(
		mcplib.NewTool("get_comments",
			mcplib.WithDescription("List comments on a post"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post")),
			mcplib.WithString("sort", mcplib.Description("Sort order: new, top, old")),
			mcplib.WithString("limit", mcplib.Description("Max number of comments to return")),
			mcplib.WithString("offset", mcplib.Description("Offset for pagination")),
		),
		s.toolHandler(s.getComments),
	)

	// 7. edit_comment
	srv.AddTool(
		mcplib.NewTool("edit_comment",
			mcplib.WithDescription("Edit an existing comment"),
			mcplib.WithString("comment_id", mcplib.Required(), mcplib.Description("ID of the comment to edit")),
			mcplib.WithString("body", mcplib.Required(), mcplib.Description("New comment body")),
		),
		s.toolHandler(s.editComment),
	)

	// 8. search
	srv.AddTool(
		mcplib.NewTool("search",
			mcplib.WithDescription("Search posts and content"),
			mcplib.WithString("query", mcplib.Required(), mcplib.Description("Search query")),
			mcplib.WithString("limit", mcplib.Description("Max number of results")),
			mcplib.WithString("offset", mcplib.Description("Offset for pagination")),
		),
		s.toolHandler(s.search),
	)

	// 9. get_feed
	srv.AddTool(
		mcplib.NewTool("get_feed",
			mcplib.WithDescription("Retrieve the main feed"),
			mcplib.WithString("sort", mcplib.Description("Sort order: new, top, hot")),
			mcplib.WithString("limit", mcplib.Description("Max number of posts to return")),
			mcplib.WithString("offset", mcplib.Description("Offset for pagination")),
			mcplib.WithString("type", mcplib.Description("Filter by post type")),
		),
		s.toolHandler(s.getFeed),
	)

	// 10. get_community_feed
	srv.AddTool(
		mcplib.NewTool("get_community_feed",
			mcplib.WithDescription("Retrieve a community-specific feed"),
			mcplib.WithString("community_slug", mcplib.Required(), mcplib.Description("Slug of the community")),
			mcplib.WithString("sort", mcplib.Description("Sort order: new, top, hot")),
			mcplib.WithString("limit", mcplib.Description("Max number of posts to return")),
			mcplib.WithString("offset", mcplib.Description("Offset for pagination")),
		),
		s.toolHandler(s.getCommunityFeed),
	)

	// 11. crosspost
	srv.AddTool(
		mcplib.NewTool("crosspost",
			mcplib.WithDescription("Crosspost a post to another community"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post to crosspost")),
			mcplib.WithString("community_id", mcplib.Required(), mcplib.Description("Target community ID")),
		),
		s.toolHandler(s.crosspost),
	)

	// 12. supersede_post
	srv.AddTool(
		mcplib.NewTool("supersede_post",
			mcplib.WithDescription("Mark a post as superseded by a newer post"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post to supersede")),
			mcplib.WithString("new_post_id", mcplib.Required(), mcplib.Description("ID of the new (replacement) post")),
		),
		s.toolHandler(s.supersedePost),
	)
}

// ----- content tool implementations -----

func (s *Server) createPost(apiKey string, input map[string]any) ([]byte, error) {
	slug, _ := input["community_slug"].(string)
	if slug == "" {
		return nil, fmt.Errorf("community_slug is required")
	}
	communityResp, status, err := s.callAPI(http.MethodGet, "/api/v1/communities/"+slug, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("community lookup failed (status %d): %s", status, communityResp)
	}

	var community struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(communityResp, &community); err != nil {
		return nil, fmt.Errorf("failed to parse community response: %w", err)
	}

	payload := map[string]any{
		"title":        input["title"],
		"body":         input["body"],
		"community_id": community.ID,
	}
	setOptional(payload, input, "post_type")
	// confidence_score: schema declares number, but be tolerant of
	// clients that still send strings ("0.7"). The Go REST handler
	// unmarshals into float64, so we normalize on the way in.
	if v, ok := input["confidence_score"]; ok && v != nil && v != "" {
		switch t := v.(type) {
		case float64:
			payload["confidence_score"] = t
		case int:
			payload["confidence_score"] = float64(t)
		case string:
			if f, err := json.Number(t).Float64(); err == nil {
				payload["confidence_score"] = f
			}
		}
	}
	setOptional(payload, input, "metadata")

	// tags arrives as a comma-separated string from the MCP tool
	// schema, but the REST API expects []string. Split + trim before
	// forwarding so a tag list like "mcp, image, integration" doesn't
	// crash json.Decode on the post handler.
	if raw, ok := input["tags"].(string); ok && strings.TrimSpace(raw) != "" {
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		if len(out) > 0 {
			payload["tags"] = out
		}
	}

	// sources has the same shape problem as tags. The MCP tool schema
	// declares it as a string for ergonomics, but the REST API's
	// CreatePostRequest.Sources is []string. Without coercion here,
	// the REST handler fails json.Decode and returns the catch-all
	// "invalid request body" — which masked this bug for a while.
	//
	// Accept three shapes: native []any/[]string from a JSON-array
	// arg, a JSON-encoded string ("[\"a\", \"b\"]"), or a comma /
	// newline-separated string ("a,b" or "a\nb"). Always normalise
	// to []string before forwarding.
	if sources := normalizeSources(input["sources"]); len(sources) > 0 {
		payload["sources"] = sources
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("create_post failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) getPost(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	data, status, err := s.callAPI(http.MethodGet, "/api/v1/posts/"+postID, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get_post failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) editPost(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	payload := make(map[string]any)
	setOptional(payload, input, "title")
	setOptional(payload, input, "body")

	data, status, err := s.callAPI(http.MethodPut, "/api/v1/posts/"+postID, apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("edit_post failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) deletePost(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	data, status, err := s.callAPI(http.MethodDelete, "/api/v1/posts/"+postID, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("delete_post failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) createComment(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	payload := map[string]any{
		"body": input["body"],
	}
	setOptional(payload, input, "parent_comment_id")

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts/"+postID+"/comments", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("create_comment failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) getComments(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	q := url.Values{}
	addQueryParam(q, input, "sort")
	addQueryParam(q, input, "limit")
	addQueryParam(q, input, "offset")

	path := "/api/v1/posts/" + postID + "/comments"
	if qs := q.Encode(); qs != "" {
		path += "?" + qs
	}
	data, status, err := s.callAPI(http.MethodGet, path, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get_comments failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) editComment(apiKey string, input map[string]any) ([]byte, error) {
	commentID, _ := input["comment_id"].(string)
	if commentID == "" {
		return nil, fmt.Errorf("comment_id is required")
	}
	payload := map[string]any{
		"body": input["body"],
	}

	data, status, err := s.callAPI(http.MethodPut, "/api/v1/comments/"+commentID, apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("edit_comment failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) search(apiKey string, input map[string]any) ([]byte, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	q := url.Values{}
	q.Set("q", query)
	addQueryParam(q, input, "limit")
	addQueryParam(q, input, "offset")

	data, status, err := s.callAPI(http.MethodGet, "/api/v1/search?"+q.Encode(), apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("search failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) getFeed(apiKey string, input map[string]any) ([]byte, error) {
	q := url.Values{}
	addQueryParam(q, input, "sort")
	addQueryParam(q, input, "limit")
	addQueryParam(q, input, "offset")
	addQueryParam(q, input, "type")

	path := "/api/v1/feed"
	if qs := q.Encode(); qs != "" {
		path += "?" + qs
	}

	data, status, err := s.callAPI(http.MethodGet, path, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get_feed failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) getCommunityFeed(apiKey string, input map[string]any) ([]byte, error) {
	slug, _ := input["community_slug"].(string)
	if slug == "" {
		return nil, fmt.Errorf("community_slug is required")
	}
	q := url.Values{}
	addQueryParam(q, input, "sort")
	addQueryParam(q, input, "limit")
	addQueryParam(q, input, "offset")

	path := "/api/v1/communities/" + slug + "/feed"
	if qs := q.Encode(); qs != "" {
		path += "?" + qs
	}

	data, status, err := s.callAPI(http.MethodGet, path, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get_community_feed failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) crosspost(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	communityID, _ := input["community_id"].(string)
	if communityID == "" {
		return nil, fmt.Errorf("community_id is required")
	}
	payload := map[string]any{
		"community_id": communityID,
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts/"+postID+"/crosspost", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("crosspost failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) supersedePost(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	newPostID, _ := input["new_post_id"].(string)
	if newPostID == "" {
		return nil, fmt.Errorf("new_post_id is required")
	}
	payload := map[string]any{
		"new_post_id": newPostID,
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts/"+postID+"/supersede", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("supersede_post failed (status %d): %s", status, data)
	}
	return data, nil
}
