package mcp

import (
	"fmt"
	"net/http"
	"net/url"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerMemoryTools(srv *mcpserver.MCPServer) {
	// 27. store_memory
	srv.AddTool(
		mcplib.NewTool("store_memory",
			mcplib.WithDescription("Store a key-value pair in persistent agent memory. Values can be any JSON."),
			mcplib.WithString("key", mcplib.Required(), mcplib.Description("Memory key (max 256 chars)")),
			mcplib.WithString("value", mcplib.Required(), mcplib.Description("JSON value to store")),
		),
		s.toolHandler(s.storeMemory),
	)

	// 28. recall_memory
	srv.AddTool(
		mcplib.NewTool("recall_memory",
			mcplib.WithDescription("Retrieve a value from agent memory by key"),
			mcplib.WithString("key", mcplib.Required(), mcplib.Description("Memory key to retrieve")),
		),
		s.toolHandler(s.recallMemory),
	)

	// 29. list_memories
	srv.AddTool(
		mcplib.NewTool("list_memories",
			mcplib.WithDescription("List all stored memory keys, optionally filtered by prefix"),
			mcplib.WithString("prefix", mcplib.Description("Key prefix to filter by")),
		),
		s.toolHandler(s.listMemories),
	)

	// 30. delete_memory
	srv.AddTool(
		mcplib.NewTool("delete_memory",
			mcplib.WithDescription("Delete a key-value pair from agent memory"),
			mcplib.WithString("key", mcplib.Required(), mcplib.Description("Memory key to delete")),
		),
		s.toolHandler(s.deleteMemory),
	)

	// 31. clear_memory
	srv.AddTool(
		mcplib.NewTool("clear_memory",
			mcplib.WithDescription("Delete all stored memory for the authenticated agent"),
		),
		s.toolHandler(s.clearMemory),
	)
}

// ----- memory tool implementations -----

func (s *Server) storeMemory(apiKey string, input map[string]any) ([]byte, error) {
	key, _ := input["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}
	value := input["value"]

	data, status, err := s.callAPI(http.MethodPut, "/api/v1/agent-memory/"+key, apiKey, value)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("store_memory failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) recallMemory(apiKey string, input map[string]any) ([]byte, error) {
	key, _ := input["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}

	data, status, err := s.callAPI(http.MethodGet, "/api/v1/agent-memory/"+key, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("recall_memory failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) listMemories(apiKey string, input map[string]any) ([]byte, error) {
	q := url.Values{}
	addQueryParam(q, input, "prefix")

	path := "/api/v1/agent-memory"
	if qs := q.Encode(); qs != "" {
		path += "?" + qs
	}

	data, status, err := s.callAPI(http.MethodGet, path, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list_memories failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) deleteMemory(apiKey string, input map[string]any) ([]byte, error) {
	key, _ := input["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}

	data, status, err := s.callAPI(http.MethodDelete, "/api/v1/agent-memory/"+key, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("delete_memory failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) clearMemory(apiKey string, input map[string]any) ([]byte, error) {
	data, status, err := s.callAPI(http.MethodDelete, "/api/v1/agent-memory", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("clear_memory failed (status %d): %s", status, data)
	}
	return data, nil
}
