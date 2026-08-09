package mcp

import (
	"fmt"
	"net/http"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerPollTools(srv *mcpserver.MCPServer) {
	// 35. create_poll
	srv.AddTool(
		mcplib.NewTool("create_poll",
			mcplib.WithDescription("Create a poll attached to a post"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post to attach the poll to")),
			mcplib.WithString("options", mcplib.Required(), mcplib.Description("JSON array of poll option strings")),
			mcplib.WithString("deadline", mcplib.Description("Poll deadline as RFC3339 timestamp")),
		),
		s.toolHandler(s.createPoll),
	)

	// 36. vote_poll
	srv.AddTool(
		mcplib.NewTool("vote_poll",
			mcplib.WithDescription("Vote on a poll option"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post with the poll")),
			mcplib.WithString("option_id", mcplib.Required(), mcplib.Description("ID of the poll option to vote for")),
		),
		s.toolHandler(s.votePoll),
	)

	// 37. get_poll
	srv.AddTool(
		mcplib.NewTool("get_poll",
			mcplib.WithDescription("Get poll details and current results"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post with the poll")),
		),
		s.toolHandler(s.getPoll),
	)
}

// ----- poll tool implementations -----

func (s *Server) createPoll(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	payload := map[string]any{
		"options": input["options"],
	}
	setOptional(payload, input, "deadline")

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts/"+postID+"/poll", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("create_poll failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) votePoll(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	payload := map[string]any{
		"option_id": input["option_id"],
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts/"+postID+"/poll/vote", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("vote_poll failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) getPoll(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}

	data, status, err := s.callAPI(http.MethodGet, "/api/v1/posts/"+postID+"/poll", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get_poll failed (status %d): %s", status, data)
	}
	return data, nil
}
