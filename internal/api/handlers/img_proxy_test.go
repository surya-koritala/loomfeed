package handlers

import (
	"bytes"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/RoamXAI/loomfeed/internal/cache"
)

func newImgTestCache(t *testing.T) (*cache.RedisCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return cache.NewRedisCache(client), mr
}

// Bodies at or under the Redis size cap are cached (bytes + content-type
// sidecar); bodies over it are NOT — they would crowd the small shared
// instance and rely on Cloudflare's edge cache instead.
func TestCacheImage_SizeCap(t *testing.T) {
	c, mr := newImgTestCache(t)
	h := &ImgProxyHandler{cache: c}

	small := bytes.Repeat([]byte{0xAB}, 10*1024)
	h.cacheImage("img:small", "img:small:ct", small, "image/png")
	if got, _ := c.Get(t.Context(), "img:small"); !bytes.Equal(got, small) {
		t.Fatalf("small body should be cached; got %d bytes", len(got))
	}
	if got, _ := c.Get(t.Context(), "img:small:ct"); string(got) != "image/png" {
		t.Fatalf("content-type sidecar should be cached; got %q", got)
	}
	if ttl := mr.TTL("img:small"); ttl <= 0 {
		t.Fatalf("cached image must carry a TTL; got %v", ttl)
	}

	big := bytes.Repeat([]byte{0xCD}, maxRedisCacheable+1)
	h.cacheImage("img:big", "img:big:ct", big, "image/jpeg")
	if got, _ := c.Get(t.Context(), "img:big"); got != nil {
		t.Fatalf("oversize body must NOT be cached; got %d bytes", len(got))
	}
	if got, _ := c.Get(t.Context(), "img:big:ct"); got != nil {
		t.Fatalf("oversize body must not cache a content-type sidecar; got %q", got)
	}

	boundary := bytes.Repeat([]byte{0xEF}, maxRedisCacheable)
	h.cacheImage("img:boundary", "img:boundary:ct", boundary, "image/jpeg")
	if got, _ := c.Get(t.Context(), "img:boundary"); !bytes.Equal(got, boundary) {
		t.Fatalf("body exactly at the cap should be cached; got %d bytes", len(got))
	}
}

// A nil cache (Redis disabled) must be a safe no-op.
func TestCacheImage_NilCache(t *testing.T) {
	h := &ImgProxyHandler{cache: nil}
	h.cacheImage("img:k", "img:k:ct", []byte("data"), "image/png") // must not panic
}
