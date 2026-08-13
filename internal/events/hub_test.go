package events_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/surya-koritala/loomfeed/internal/events"
)

func TestHubConcurrentPublishAndUnsubscribe(t *testing.T) {
	const rounds = 2_000
	hub := events.NewHub()

	for round := 0; round < rounds; round++ {
		ch := hub.Subscribe("participant")
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(3)
		for publisher := 0; publisher < 2; publisher++ {
			publisher := publisher
			go func() {
				defer wg.Done()
				<-start
				for attempt := 0; attempt < 8; attempt++ {
					if publisher == 0 {
						hub.Publish("participant", events.Event{Type: "test", Data: "{}"})
					} else {
						hub.Broadcast(events.Event{Type: "broadcast", Data: "{}"})
					}
				}
			}()
		}
		go func() {
			defer wg.Done()
			<-start
			hub.Unsubscribe("participant", ch)
		}()
		close(start)
		wg.Wait()
	}
}

func TestHubSlowSubscriberDropsNewestEvent(t *testing.T) {
	hub := events.NewHub()
	ch := hub.Subscribe("participant")
	defer hub.Unsubscribe("participant", ch)

	for sequence := 0; sequence < 17; sequence++ {
		hub.Publish("participant", events.Event{Type: "sequence", Data: string(rune('a' + sequence))})
	}
	for sequence := 0; sequence < 16; sequence++ {
		want := events.Event{Type: "sequence", Data: string(rune('a' + sequence))}
		assertEvent(t, ch, want)
	}
	select {
	case event := <-ch:
		t.Fatalf("slow subscriber received event beyond its buffer: %+v", event)
	default:
	}
}

func TestHubCloseOwnsSubscriberChannelClosure(t *testing.T) {
	hub := events.NewHub()
	ch := hub.Subscribe("participant")
	if err := hub.Close(); err != nil {
		t.Fatalf("close hub: %v", err)
	}
	if _, ok := <-ch; ok {
		t.Fatal("subscriber channel remained open after hub close")
	}
	// A handler cleanup racing or following Hub.Close is idempotent.
	hub.Unsubscribe("participant", ch)
}

func TestHubConcurrentPublishUnsubscribeAndClose(t *testing.T) {
	const rounds = 500
	for round := 0; round < rounds; round++ {
		hub := events.NewHub()
		ch := hub.Subscribe("participant")
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(3)
		go func() {
			defer wg.Done()
			<-start
			for attempt := 0; attempt < 8; attempt++ {
				hub.Publish("participant", events.Event{Type: "test", Data: "{}"})
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			hub.Unsubscribe("participant", ch)
		}()
		go func() {
			defer wg.Done()
			<-start
			if err := hub.Close(); err != nil {
				t.Errorf("close hub: %v", err)
			}
		}()
		close(start)
		wg.Wait()
	}
}

func TestRedisHubPublishesOnceLocallyAndAcrossInstances(t *testing.T) {
	redisServer := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = clientA.Close()
		_ = clientB.Close()
	})

	hubA, err := events.NewRedisHub(context.Background(), clientA)
	if err != nil {
		t.Fatalf("create first Redis hub: %v", err)
	}
	defer hubA.Close()
	hubB, err := events.NewRedisHub(context.Background(), clientB)
	if err != nil {
		t.Fatalf("create second Redis hub: %v", err)
	}
	defer hubB.Close()

	local := hubA.Subscribe("participant")
	defer hubA.Unsubscribe("participant", local)
	remote := hubB.Subscribe("participant")
	defer hubB.Unsubscribe("participant", remote)
	want := events.Event{Type: "mention", Data: `{"comment_id":"comment-1"}`}
	hubA.Publish("participant", want)

	assertEvent(t, local, want)
	assertEvent(t, remote, want)
	select {
	case duplicate := <-local:
		t.Fatalf("origin hub received duplicate broker event: %+v", duplicate)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRedisHubPublishLocalStaysOnOriginInstance(t *testing.T) {
	redisServer := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer clientA.Close()
	defer clientB.Close()
	hubA, err := events.NewRedisHub(context.Background(), clientA)
	if err != nil {
		t.Fatalf("create first Redis hub: %v", err)
	}
	defer hubA.Close()
	hubB, err := events.NewRedisHub(context.Background(), clientB)
	if err != nil {
		t.Fatalf("create second Redis hub: %v", err)
	}
	defer hubB.Close()

	local := hubA.Subscribe("__worker__")
	defer hubA.Unsubscribe("__worker__", local)
	remote := hubB.Subscribe("__worker__")
	defer hubB.Unsubscribe("__worker__", remote)
	want := events.Event{Type: "work", Data: `{"id":"job-1"}`}
	hubA.PublishLocal("__worker__", want)
	assertEvent(t, local, want)
	select {
	case event := <-remote:
		t.Fatalf("process-local event crossed replicas: %+v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRedisHubStillDeliversLocallyWhenBrokerIsUnavailable(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer client.Close()
	hub, err := events.NewRedisHub(context.Background(), client)
	if err != nil {
		t.Fatalf("create Redis hub: %v", err)
	}
	defer hub.Close()

	local := hub.Subscribe("participant")
	defer hub.Unsubscribe("participant", local)
	redisServer.Close()
	want := events.Event{Type: "mention", Data: `{"comment_id":"comment-1"}`}
	hub.Publish("participant", want)
	assertEvent(t, local, want)
}

func assertEvent(t *testing.T, ch <-chan events.Event, want events.Event) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("event=%+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for event %+v", want)
	}
}
