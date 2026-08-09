package mcp

import (
	"fmt"
	"net/http"
	"net/url"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerNotificationTools(srv *mcpserver.MCPServer) {
	// 42. get_notifications
	srv.AddTool(
		mcplib.NewTool("get_notifications",
			mcplib.WithDescription("Get notifications for the authenticated participant"),
			mcplib.WithString("limit", mcplib.Description("Max number of notifications to return")),
			mcplib.WithString("offset", mcplib.Description("Offset for pagination")),
		),
		s.toolHandler(s.getNotifications),
	)

	// 43. unread_count
	srv.AddTool(
		mcplib.NewTool("unread_count",
			mcplib.WithDescription("Get the count of unread notifications"),
		),
		s.toolHandler(s.unreadCount),
	)

	// 44. mark_notification_read
	srv.AddTool(
		mcplib.NewTool("mark_notification_read",
			mcplib.WithDescription("Mark a single notification as read"),
			mcplib.WithString("notification_id", mcplib.Required(), mcplib.Description("ID of the notification")),
		),
		s.toolHandler(s.markNotificationRead),
	)

	// 45. mark_all_read
	srv.AddTool(
		mcplib.NewTool("mark_all_read",
			mcplib.WithDescription("Mark all notifications as read"),
		),
		s.toolHandler(s.markAllRead),
	)
}

// ----- notification tool implementations -----

func (s *Server) getNotifications(apiKey string, input map[string]any) ([]byte, error) {
	q := url.Values{}
	addQueryParam(q, input, "limit")
	addQueryParam(q, input, "offset")

	path := "/api/v1/notifications"
	if qs := q.Encode(); qs != "" {
		path += "?" + qs
	}

	data, status, err := s.callAPI(http.MethodGet, path, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get_notifications failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) unreadCount(apiKey string, input map[string]any) ([]byte, error) {
	data, status, err := s.callAPI(http.MethodGet, "/api/v1/notifications/unread-count", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("unread_count failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) markNotificationRead(apiKey string, input map[string]any) ([]byte, error) {
	notifID, _ := input["notification_id"].(string)
	if notifID == "" {
		return nil, fmt.Errorf("notification_id is required")
	}

	data, status, err := s.callAPI(http.MethodPut, "/api/v1/notifications/"+notifID+"/read", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("mark_notification_read failed (status %d): %s", status, data)
	}
	return data, nil
}

func (s *Server) markAllRead(apiKey string, input map[string]any) ([]byte, error) {
	data, status, err := s.callAPI(http.MethodPut, "/api/v1/notifications/read-all", apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("mark_all_read failed (status %d): %s", status, data)
	}
	return data, nil
}
