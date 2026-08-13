package handlers

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/events"
)

func TestSSEStreamsSurviveServerWriteTimeout(t *testing.T) {
	for _, test := range sseHandlerTests() {
		t.Run(test.name, func(t *testing.T) {
			hub := events.NewHub()
			handler := NewEventHandler(hub, &config.Config{})
			mux := http.NewServeMux()
			test.register(mux, handler)

			server := httptest.NewUnstartedServer(mux)
			server.Config.WriteTimeout = 50 * time.Millisecond
			server.Start()
			defer server.Close()

			response, err := server.Client().Get(server.URL + test.path)
			if err != nil {
				t.Fatalf("open SSE stream: %v", err)
			}
			defer response.Body.Close()
			reader := bufio.NewReader(response.Body)
			if frame := readSSEFrame(t, reader); !strings.Contains(frame, "event: connected") {
				t.Fatalf("initial frame=%q", frame)
			}

			// The next write happens after the HTTP server deadline. SSE handlers
			// must clear it so the long-lived response still works.
			time.Sleep(150 * time.Millisecond)
			hub.Publish(test.key, events.Event{Type: "comment.created", Data: `{"comment_id":"comment-1"}`})

			frame := readSSEFrame(t, reader)
			if !strings.Contains(frame, "event: comment.created") || !strings.Contains(frame, `"comment_id":"comment-1"`) {
				t.Fatalf("event frame=%q", frame)
			}
		})
	}
}

func TestSSEStreamsSendHeartbeatComments(t *testing.T) {
	for _, test := range sseHandlerTests() {
		t.Run(test.name, func(t *testing.T) {
			hub := events.NewHub()
			handler := NewEventHandler(hub, &config.Config{})
			handler.heartbeatInterval = 10 * time.Millisecond
			mux := http.NewServeMux()
			test.register(mux, handler)
			server := httptest.NewServer(mux)
			defer server.Close()

			response, err := server.Client().Get(server.URL + test.path)
			if err != nil {
				t.Fatalf("open SSE stream: %v", err)
			}
			defer response.Body.Close()
			reader := bufio.NewReader(response.Body)
			_ = readSSEFrame(t, reader)

			if frame := readSSEFrame(t, reader); frame != ": heartbeat\n\n" {
				t.Fatalf("heartbeat frame=%q", frame)
			}
		})
	}
}

type sseHandlerTest struct {
	name     string
	path     string
	key      string
	register func(*http.ServeMux, *EventHandler)
}

func sseHandlerTests() []sseHandlerTest {
	return []sseHandlerTest{
		{
			name: "participant stream",
			path: "/api/v1/events/stream",
			key:  "participant-1",
			register: func(mux *http.ServeMux, handler *EventHandler) {
				mux.HandleFunc("GET /api/v1/events/stream", func(w http.ResponseWriter, r *http.Request) {
					claims := &auth.Claims{ParticipantID: "participant-1"}
					r = r.WithContext(context.WithValue(r.Context(), middleware.ClaimsKey, claims))
					handler.Stream(w, r)
				})
			},
		},
		{
			name: "post stream",
			path: "/api/v1/events/post/post-1",
			key:  "post:post-1",
			register: func(mux *http.ServeMux, handler *EventHandler) {
				mux.HandleFunc("GET /api/v1/events/post/{id}", handler.PostStream)
			},
		},
	}
}

func readSSEFrame(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	type result struct {
		frame string
		err   error
	}
	resultCh := make(chan result, 1)
	go func() {
		var frame strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				resultCh <- result{err: err}
				return
			}
			frame.WriteString(line)
			if line == "\n" {
				resultCh <- result{frame: frame.String()}
				return
			}
		}
	}()
	select {
	case got := <-resultCh:
		if got.err != nil {
			if got.err == io.EOF {
				t.Fatalf("SSE stream closed before the next frame")
			}
			t.Fatalf("read SSE frame: %v", got.err)
		}
		return got.frame
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SSE frame")
		return ""
	}
}
