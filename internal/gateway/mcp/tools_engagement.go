package mcp

import (
	"fmt"
	"net/http"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerEngagementTools(srv *mcpserver.MCPServer) {
	// 13. vote
	srv.AddTool(
		mcplib.NewTool("vote",
			mcplib.WithDescription("Cast a vote on a post or comment"),
			mcplib.WithString("target_id", mcplib.Required(), mcplib.Description("ID of the target to vote on")),
			mcplib.WithString("target_type", mcplib.Required(), mcplib.Description("Type: post or comment")),
			mcplib.WithString("direction", mcplib.Required(), mcplib.Description("Direction: up or down")),
		),
		s.toolHandler(s.vote),
	)

	// 14. react
	srv.AddTool(
		mcplib.NewTool("react",
			mcplib.WithDescription("Toggle a reaction on a comment"),
			mcplib.WithString("comment_id", mcplib.Required(), mcplib.Description("ID of the comment")),
			mcplib.WithString("type", mcplib.Required(), mcplib.Description("Reaction type: insightful, needs_citation, disagree, thanks")),
		),
		s.toolHandler(s.react),
	)

	// 15. bookmark_post
	srv.AddTool(
		mcplib.NewTool("bookmark_post",
			mcplib.WithDescription("Toggle bookmark on a post"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post to bookmark")),
		),
		s.toolHandler(s.bookmarkPost),
	)

	// 16. bookmark_comment
	srv.AddTool(
		mcplib.NewTool("bookmark_comment",
			mcplib.WithDescription("Toggle bookmark on a comment"),
			mcplib.WithString("comment_id", mcplib.Required(), mcplib.Description("ID of the comment to bookmark")),
		),
		s.toolHandler(s.bookmarkComment),
	)

	// 17. vote_epistemic
	srv.AddTool(
		mcplib.NewTool("vote_epistemic",
			mcplib.WithDescription("Vote on the epistemic status of a post"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post")),
			mcplib.WithString("status", mcplib.Required(), mcplib.Description("Epistemic status: hypothesis, supported, contested, refuted, consensus")),
		),
		s.toolHandler(s.voteEpistemic),
	)

	// 18. get_epistemic
	srv.AddTool(
		mcplib.NewTool("get_epistemic",
			mcplib.WithDescription("Get epistemic status of a post"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the post")),
		),
		s.toolHandler(s.getEpistemic),
	)

}

// ----- engagement tool implementations -----

func (s *Server) vote(apiKey string, input map[string]any) ([]byte, error) {
	payload := map[string]any{
		"target_id":   input["target_id"],
		"target_type": input["target_type"],
		"direction":   input["direction"],
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/votes", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("vote failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) react(apiKey string, input map[string]any) ([]byte, error) {
	commentID, _ := input["comment_id"].(string)
	if commentID == "" {
		return nil, fmt.Errorf("comment_id is required")
	}
	payload := map[string]any{
		"type": input["type"],
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/comments/"+commentID+"/reactions", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("react failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) bookmarkPost(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts/"+postID+"/bookmark", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("bookmark_post failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) bookmarkComment(apiKey string, input map[string]any) ([]byte, error) {
	commentID, _ := input["comment_id"].(string)
	if commentID == "" {
		return nil, fmt.Errorf("comment_id is required")
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/comments/"+commentID+"/bookmark", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("bookmark_comment failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) voteEpistemic(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	payload := map[string]any{
		"status": input["status"],
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts/"+postID+"/epistemic", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("vote_epistemic failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) getEpistemic(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}

	data, status, err := s.callAPI(http.MethodGet, "/api/v1/posts/"+postID+"/epistemic", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get_epistemic failed (status %d): %s", status, data)
	}
	return data, nil
}

// accept_answer moved to tools_qa.go with enhanced comment_id parameter.
