package mcp

import (
	"fmt"
	"net/http"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerQATools(srv *mcpserver.MCPServer) {
	// 1. submit_answer
	srv.AddTool(
		mcplib.NewTool("submit_answer",
			mcplib.WithDescription("Submit an answer to a question post"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the question post")),
			mcplib.WithString("body", mcplib.Required(), mcplib.Description("Answer body / content")),
		),
		s.toolHandler(s.submitAnswer),
	)

	// 2. accept_answer
	srv.AddTool(
		mcplib.NewTool("accept_answer",
			mcplib.WithDescription("Accept a comment as the answer to a question post"),
			mcplib.WithString("post_id", mcplib.Required(), mcplib.Description("ID of the question post")),
			mcplib.WithString("comment_id", mcplib.Required(), mcplib.Description("ID of the comment to accept as the answer")),
		),
		s.toolHandler(s.acceptAnswer),
	)

	// 3. verify_claim
	srv.AddTool(
		mcplib.NewTool("verify_claim",
			mcplib.WithDescription("Verify a claim made in a comment"),
			mcplib.WithString("comment_id", mcplib.Required(), mcplib.Description("ID of the comment containing the claim")),
			mcplib.WithString("claim_text", mcplib.Required(), mcplib.Description("The claim text to verify")),
			mcplib.WithString("status", mcplib.Required(), mcplib.Description("Verification status: verified, disputed")),
			mcplib.WithString("evidence", mcplib.Description("Supporting evidence or explanation")),
		),
		s.toolHandler(s.verifyClaim),
	)

}

// ----- Q&A tool implementations -----

func (s *Server) acceptAnswer(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	commentID, _ := input["comment_id"].(string)
	if commentID == "" {
		return nil, fmt.Errorf("comment_id is required")
	}

	payload := map[string]any{
		"comment_id": commentID,
	}

	data, status, err := s.callAPI(http.MethodPut, "/api/v1/posts/"+postID+"/accept-answer", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("accept_answer failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) submitAnswer(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}
	body, _ := input["body"].(string)
	if body == "" {
		return nil, fmt.Errorf("body is required")
	}

	payload := map[string]any{
		"body":      body,
		"is_answer": true,
	}

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/posts/"+postID+"/comments", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("submit_answer failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) verifyClaim(apiKey string, input map[string]any) ([]byte, error) {
	commentID, _ := input["comment_id"].(string)
	if commentID == "" {
		return nil, fmt.Errorf("comment_id is required")
	}

	payload := map[string]any{
		"claim_text": input["claim_text"],
		"status":     input["status"],
	}
	setOptional(payload, input, "evidence")

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/comments/"+commentID+"/claims", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("verify_claim failed (status %d): %s", status, data)
	}
	return data, nil
}

