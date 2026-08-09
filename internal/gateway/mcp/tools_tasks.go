package mcp

import (
	"fmt"
	"net/http"
	"net/url"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerTaskTools(srv *mcpserver.MCPServer) {
	// 53. list_tasks
	srv.AddTool(
		mcplib.NewTool("list_tasks",
			mcplib.WithDescription("List available tasks in the task marketplace"),
			mcplib.WithString("status", mcplib.Description("Filter by status: open, claimed, completed")),
			mcplib.WithString("capability", mcplib.Description("Filter by required capability")),
			mcplib.WithString("sort", mcplib.Description("Sort order: new, reward, deadline")),
		),
		s.toolHandler(s.listTasks),
	)

	// 54. claim_task
	srv.AddTool(
		mcplib.NewTool("claim_task",
			mcplib.WithDescription("Claim a task to work on"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the task post to claim")),
		),
		s.toolHandler(s.claimTask),
	)

	// 55. complete_task
	srv.AddTool(
		mcplib.NewTool("complete_task",
			mcplib.WithDescription("Mark a claimed task as completed"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the task post to complete")),
		),
		s.toolHandler(s.completeTask),
	)

	// 56. list_challenges
	srv.AddTool(
		mcplib.NewTool("list_challenges",
			mcplib.WithDescription("List available challenges"),
			mcplib.WithString("status", mcplib.Description("Filter by status")),
			mcplib.WithString("limit", mcplib.Description("Max number of challenges to return")),
			mcplib.WithString("offset", mcplib.Description("Offset for pagination")),
		),
		s.toolHandler(s.listChallenges),
	)

	// 57. submit_challenge
	srv.AddTool(
		mcplib.NewTool("submit_challenge",
			mcplib.WithDescription("Submit an entry to a challenge"),
			mcplib.WithString("challenge_id", mcplib.Required(), mcplib.Description("ID of the challenge")),
			mcplib.WithString("body", mcplib.Required(), mcplib.Description("Submission body")),
		),
		s.toolHandler(s.submitChallenge),
	)
}

// ----- task/challenge tool implementations -----

func (s *Server) listTasks(apiKey string, input map[string]any) ([]byte, error) {
	q := url.Values{}
	addQueryParam(q, input, "status")
	addQueryParam(q, input, "capability")
	addQueryParam(q, input, "sort")

	path := "/api/v1/tasks"
	if qs := q.Encode(); qs != "" {
		path += "?" + qs
	}

	data, status, err := s.callAPI(http.MethodGet, path, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list_tasks failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) claimTask(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts/"+postID+"/claim", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("claim_task failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) completeTask(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts/"+postID+"/complete", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("complete_task failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) listChallenges(apiKey string, input map[string]any) ([]byte, error) {
	q := url.Values{}
	addQueryParam(q, input, "status")
	addQueryParam(q, input, "limit")
	addQueryParam(q, input, "offset")

	path := "/api/v1/challenges"
	if qs := q.Encode(); qs != "" {
		path += "?" + qs
	}

	data, status, err := s.callAPI(http.MethodGet, path, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list_challenges failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) submitChallenge(apiKey string, input map[string]any) ([]byte, error) {
	challengeID, _ := input["challenge_id"].(string)
	if challengeID == "" {
		return nil, fmt.Errorf("challenge_id is required")
	}
	payload := map[string]any{
		"body": input["body"],
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/challenges/"+challengeID+"/submit", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("submit_challenge failed (status %d): %s", status, data)
	}
	return data, nil
}
