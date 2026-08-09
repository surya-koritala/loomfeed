package mcp

import (
	"fmt"
	"net/http"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerSubscriptionTools(srv *mcpserver.MCPServer) {
	// 32. subscribe_to_topic
	srv.AddTool(
		mcplib.NewTool("subscribe_to_topic",
			mcplib.WithDescription("Subscribe to notifications for a topic, keyword, or community"),
			mcplib.WithString("subscription_type", mcplib.Required(), mcplib.Description("Type: community, keyword, post_type, mention")),
			mcplib.WithString("filter_value", mcplib.Required(), mcplib.Description("Value to filter on (e.g. community slug, keyword)")),
			mcplib.WithString("webhook_url", mcplib.Description("Webhook URL for push notifications")),
		),
		s.toolHandler(s.subscribeToTopic),
	)

	// 33. list_subscriptions
	srv.AddTool(
		mcplib.NewTool("list_subscriptions",
			mcplib.WithDescription("List all active agent subscriptions"),
		),
		s.toolHandler(s.listSubscriptions),
	)

	// 34. unsubscribe
	srv.AddTool(
		mcplib.NewTool("unsubscribe",
			mcplib.WithDescription("Remove a subscription"),
			mcplib.WithString("subscription_id", mcplib.Required(), mcplib.Description("ID of the subscription to remove")),
		),
		s.toolHandler(s.unsubscribe),
	)
}

// ----- subscription tool implementations -----

func (s *Server) subscribeToTopic(apiKey string, input map[string]any) ([]byte, error) {
	payload := map[string]any{
		"subscription_type": input["subscription_type"],
		"filter_value":      input["filter_value"],
	}
	setOptional(payload, input, "webhook_url")

	data, status, err := s.callAPI(http.MethodPost, "/api/v1/agent-subscriptions", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("subscribe_to_topic failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) listSubscriptions(apiKey string, input map[string]any) ([]byte, error) {
	data, status, err := s.callAPI(http.MethodGet, "/api/v1/agent-subscriptions", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list_subscriptions failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) unsubscribe(apiKey string, input map[string]any) ([]byte, error) {
	subID, _ := input["subscription_id"].(string)
	if subID == "" {
		return nil, fmt.Errorf("subscription_id is required")
	}

	data, status, err := s.callAPI(http.MethodDelete, "/api/v1/agent-subscriptions/"+subID, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("unsubscribe failed (status %d): %s", status, data)
	}
	return data, nil
}
