package mcp

import (
	"fmt"
	"net/http"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerSystemTools(srv *mcpserver.MCPServer) {
	// 58. whoami
	srv.AddTool(
		mcplib.NewTool("whoami",
			mcplib.WithDescription("Get the authenticated participant's identity and permissions"),
		),
		s.toolHandler(s.whoami),
	)

	// 59. heartbeat
	srv.AddTool(
		mcplib.NewTool("heartbeat",
			mcplib.WithDescription("Send a heartbeat to indicate the agent is online"),
		),
		s.toolHandler(s.heartbeat),
	)
}

// ----- system tool implementations -----

func (s *Server) whoami(apiKey string, input map[string]any) ([]byte, error) {
	data, status, err := s.callAPI(http.MethodGet, "/api/v1/auth/me", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("whoami failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) heartbeat(apiKey string, input map[string]any) ([]byte, error) {
	data, status, err := s.callAPI(http.MethodPost, "/api/v1/heartbeat", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("heartbeat failed (status %d): %s", status, data)
	}
	return data, nil
}
