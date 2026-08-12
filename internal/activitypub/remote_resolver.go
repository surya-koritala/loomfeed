package activitypub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/safehttp"
)

var webFingerClient = safehttp.NewClient(safehttp.Options{Timeout: 10 * time.Second, MaxRedirects: 3})

type ActorResolver struct {
	cache      *RemoteActorCache
	lookup     func(context.Context, string) (string, error)
	fetchActor func(context.Context, string) (*RemoteActor, error)
}

type ActorResolverOption func(*ActorResolver)

func WithWebFingerLookup(lookup func(context.Context, string) (string, error)) ActorResolverOption {
	return func(resolver *ActorResolver) {
		if lookup != nil {
			resolver.lookup = lookup
		}
	}
}

func WithRemoteActorFetcher(fetch func(context.Context, string) (*RemoteActor, error)) ActorResolverOption {
	return func(resolver *ActorResolver) {
		if fetch != nil {
			resolver.fetchActor = fetch
		}
	}
}

func NewActorResolver(pool *pgxpool.Pool, options ...ActorResolverOption) *ActorResolver {
	resolver := &ActorResolver{
		cache:      NewRemoteActorCache(pool, time.Hour),
		lookup:     lookupWebFinger,
		fetchActor: FetchActorContext,
	}
	for _, option := range options {
		option(resolver)
	}
	return resolver
}

// ParseRemoteActorReference accepts either a direct http(s) actor URI or an
// acct handle such as @alice@example.social. Exactly one return value is set.
func ParseRemoteActorReference(reference string) (actorURI, acct string, err error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return "", "", fmt.Errorf("remote actor reference is required")
	}
	if strings.HasPrefix(reference, "http://") || strings.HasPrefix(reference, "https://") {
		if err := safehttp.ValidateURL(reference); err != nil {
			return "", "", fmt.Errorf("invalid remote actor URI: %w", err)
		}
		return reference, "", nil
	}
	reference = strings.TrimPrefix(reference, "acct:")
	reference = strings.TrimPrefix(reference, "@")
	separator := strings.LastIndex(reference, "@")
	if separator <= 0 || separator == len(reference)-1 {
		return "", "", fmt.Errorf("remote actor must be an actor URL or @user@domain")
	}
	username := strings.ToLower(strings.TrimSpace(reference[:separator]))
	host := strings.ToLower(strings.TrimSpace(reference[separator+1:]))
	if username == "" || strings.ContainsAny(username, "/?#") {
		return "", "", fmt.Errorf("invalid remote actor username")
	}
	probe := "https://" + host
	parsed, parseErr := url.Parse(probe)
	if parseErr != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("invalid remote actor domain")
	}
	if err := safehttp.ValidateURL(probe); err != nil {
		return "", "", fmt.Errorf("invalid remote actor domain: %w", err)
	}
	return "", "acct:" + username + "@" + host, nil
}

func (r *ActorResolver) Resolve(ctx context.Context, reference string) (*RemoteActor, error) {
	actorURI, acct, err := ParseRemoteActorReference(reference)
	if err != nil {
		return nil, err
	}
	if acct != "" {
		if actor, found, err := r.cache.GetByAcct(ctx, strings.ToLower(acct)); err != nil {
			return nil, err
		} else if found {
			return actor, nil
		}
		actorURI, err = r.lookup(ctx, acct)
		if err != nil {
			return nil, fmt.Errorf("resolve remote actor via WebFinger: %w", err)
		}
	} else if actor, found, err := r.cache.Get(ctx, actorURI); err != nil {
		return nil, err
	} else if found {
		return actor, nil
	}

	if err := safehttp.ValidateURL(actorURI); err != nil {
		return nil, fmt.Errorf("invalid discovered actor URI: %w", err)
	}
	actor, err := r.fetchActor(ctx, actorURI)
	if err != nil {
		return nil, fmt.Errorf("fetch discovered actor: %w", err)
	}
	if err := validateRemoteActor(actor); err != nil {
		return nil, err
	}
	if err := r.cache.Put(ctx, strings.ToLower(acct), actor); err != nil {
		return nil, err
	}
	return actor, nil
}

func validateRemoteActor(actor *RemoteActor) error {
	if actor == nil || strings.TrimSpace(actor.ID) == "" {
		return fmt.Errorf("remote actor document is missing id")
	}
	if err := safehttp.ValidateURL(actor.ID); err != nil {
		return fmt.Errorf("remote actor id is unsafe: %w", err)
	}
	if strings.TrimSpace(actor.Inbox) == "" {
		return fmt.Errorf("remote actor document is missing inbox")
	}
	if err := safehttp.ValidateURL(actor.Inbox); err != nil {
		return fmt.Errorf("remote actor inbox is unsafe: %w", err)
	}
	return nil
}

func lookupWebFinger(ctx context.Context, acct string) (string, error) {
	trimmed := strings.TrimPrefix(acct, "acct:")
	separator := strings.LastIndex(trimmed, "@")
	if separator <= 0 || separator == len(trimmed)-1 {
		return "", fmt.Errorf("invalid acct identifier")
	}
	host := trimmed[separator+1:]
	endpoint := "https://" + host + "/.well-known/webfinger?resource=" + url.QueryEscape(acct)
	if err := safehttp.ValidateURL(endpoint); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build WebFinger request: %w", err)
	}
	req.Header.Set("Accept", "application/jrd+json, application/json")
	req.Header.Set("User-Agent", "loomfeed/1.0 (+https://www.loomfeed.com)")
	response, err := webFingerClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch WebFinger: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("WebFinger returned HTTP %d", response.StatusCode)
	}
	var document struct {
		Links []struct {
			Rel  string `json:"rel"`
			Type string `json:"type"`
			Href string `json:"href"`
		} `json:"links"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&document); err != nil {
		return "", fmt.Errorf("decode WebFinger: %w", err)
	}
	for _, link := range document.Links {
		if link.Rel == "self" && link.Href != "" &&
			(link.Type == "application/activity+json" || strings.Contains(link.Type, "activitystreams")) {
			if err := safehttp.ValidateURL(link.Href); err != nil {
				return "", fmt.Errorf("unsafe WebFinger actor link: %w", err)
			}
			return link.Href, nil
		}
	}
	return "", fmt.Errorf("WebFinger document has no ActivityPub self link")
}
