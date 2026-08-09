package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"  // register GIF decoder (we skip resizing GIFs, but DecodeConfig must recognise them)
	_ "image/png"  // register PNG decoder
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // register WebP decoder (input only — encode stays JPEG, no cgo)

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/cache"
	"github.com/RoamXAI/loomfeed/internal/safehttp"
)

// maxResizeArea caps the source pixel count we'll fully decode (≈12MP,
// e.g. 4000×3000 — far larger than any feed cover) so a malicious or
// huge image can't blow up memory during decode/resize. Anything
// bigger is served unresized.
const maxResizeArea = 12_000_000

// maxRedisCacheable caps what the proxy stores in Redis. The shared
// 250MB instance also holds rate-limit counters; measured 2026-06-12,
// unbounded image bodies had filled 161MB of it (the 118 keys above
// 256KB held 100MB on their own) and volatile-lru was evicting the
// limiter keys. Bodies over the cap are still served — and still ride
// Cloudflare's 7-day edge cache (s-maxage) — they just don't occupy
// Redis. Resized card variants (the high-hit-rate keys) encode well
// under this.
const maxRedisCacheable = 256 * 1024

// resizeSem bounds how many image decodes/resizes run at once. The
// per-image area cap limits a SINGLE decode, but this is a public
// endpoint with attacker-chosen dimensions — without an aggregate gate
// a burst of cold-cache ?w= requests could multiply per-request pixel
// buffers into an OOM. Sized to GOMAXPROCS, which on Go 1.25 tracks the
// container's CPU limit.
var resizeSem chan struct{}

func init() {
	n := runtime.GOMAXPROCS(0)
	if n < 2 {
		n = 2
	}
	resizeSem = make(chan struct{}, n)
}

// resizeJPEG downscales raster image bytes to at most maxW wide and
// re-encodes as JPEG. Pure-Go (no cgo): decodes jpeg/png/gif/webp via
// the registered decoders, scales with CatmullRom, encodes JPEG. It is
// conservative — it returns the ORIGINAL bytes unchanged (ok=false)
// whenever resizing wouldn't help or could go wrong:
//   - maxW <= 0 (caller didn't ask)
//   - SVG (vector) or GIF (would drop animation)
//   - undecodable / unknown format
//   - source already <= maxW (never upscale)
//   - source larger than maxResizeArea (bomb guard)
//   - the re-encoded result isn't actually smaller
func resizeJPEG(data []byte, ct string, maxW, quality int) (out []byte, outCT string, ok bool) {
	// Decoding untrusted image bytes: if any decoder panics on a
	// malformed/crafted input, fall back to serving the original.
	out, outCT, ok = data, ct, false
	defer func() {
		if recover() != nil {
			out, outCT, ok = data, ct, false
		}
	}()
	if maxW <= 0 {
		return data, ct, false
	}
	if strings.HasPrefix(ct, "image/svg") || ct == "image/gif" {
		return data, ct, false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return data, ct, false
	}
	if cfg.Width <= maxW {
		return data, ct, false // don't upscale
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxResizeArea {
		return data, ct, false
	}
	// Bound concurrent decodes (memory). If every slot is busy, serve
	// the original rather than pile on another large allocation.
	select {
	case resizeSem <- struct{}{}:
		defer func() { <-resizeSem }()
	default:
		return data, ct, false
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, ct, false
	}
	b := src.Bounds()
	// Defense in depth: the cap above used DecodeConfig; re-check the
	// ACTUALLY-decoded bounds before allocating the destination buffer,
	// and guard div-by-zero on the aspect math.
	if b.Dx() <= 0 || b.Dy() <= 0 || int64(b.Dx())*int64(b.Dy()) > maxResizeArea {
		return data, ct, false
	}
	newH := b.Dy() * maxW / b.Dx()
	if newH < 1 {
		newH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, maxW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
		return data, ct, false
	}
	if buf.Len() >= len(data) {
		return data, ct, false // re-encode didn't help (rare for downscales)
	}
	return buf.Bytes(), "image/jpeg", true
}

// ImgProxyHandler proxies external images through our origin so we
// can cache them on Cloudflare's edge and serve them at edge speed
// instead of waiting on the source CDN. Without this, post cards
// hot-link images directly from news sites — measured at 2+ seconds
// per image for slow origins like krebsonsecurity.com — which makes
// the feed feel laggy even when our own API is sub-200ms.
//
// Flow:
//
//   1. Client requests GET /api/v1/img?url=<encoded-external-url>
//   2. Handler checks Redis for cached bytes
//   3. Cache hit  → write bytes with long Cache-Control, done
//   4. Cache miss → fetch from origin, store in Redis (24h TTL),
//                   serve to client with long Cache-Control
//
// Cloudflare's Cache Rule (when configured) caches our response on
// the edge for s-maxage. So 99% of image requests after first fetch
// never hit our origin at all — they're served by the closest
// Cloudflare POP in <50ms.
type ImgProxyHandler struct {
	cache  *cache.RedisCache
	client *http.Client
}

func NewImgProxyHandler(c *cache.RedisCache) *ImgProxyHandler {
	return &ImgProxyHandler{
		cache: c,
		// SSRF-hardened at dial time (covers DNS-rebinding and redirects to
		// internal IPs, which the up-front ValidateWebhookURL check cannot).
		client: safehttp.NewClient(safehttp.Options{Timeout: 8 * time.Second, MaxRedirects: 3}),
	}
}

// isLikelyImageURL filters obvious non-image targets before we
// fetch. Final content-type validation happens after the request
// returns (we reject non-image MIMEs even if the path looked OK).
func isLikelyImageURL(u *url.URL) bool {
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" {
		return false
	}
	lower := strings.ToLower(u.Path)
	for _, bad := range []string{".html", ".php", ".asp", ".json", ".xml", ".js", ".css"} {
		if strings.HasSuffix(lower, bad) {
			return false
		}
	}
	return true
}

// cacheImage stores the proxied bytes (and a content-type sidecar) in
// Redis, best-effort — a Redis hiccup must not fail the response.
// Bodies over maxRedisCacheable are skipped entirely.
func (h *ImgProxyHandler) cacheImage(key, ctKey string, body []byte, ct string) {
	if h.cache == nil || len(body) > maxRedisCacheable {
		return
	}
	_ = h.cache.Set(context.Background(), key, body, 24*time.Hour)
	_ = h.cache.Set(context.Background(), ctKey, []byte(ct), 24*time.Hour)
}

func (h *ImgProxyHandler) Get(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")
	if raw == "" {
		api.Error(w, http.StatusBadRequest, "url is required")
		return
	}
	target, err := url.Parse(raw)
	if err != nil {
		api.Error(w, http.StatusBadRequest, "invalid url")
		return
	}
	if !isLikelyImageURL(target) {
		api.Error(w, http.StatusBadRequest, "url does not look like an image")
		return
	}
	// SSRF guard: the proxy fetches an attacker-controlled URL, so reject
	// anything resolving to private/loopback/link-local addresses. Reuses
	// the webhook validator (validates scheme, resolves DNS, checks IPs).
	// Legit external images are public; our own assets bypass the proxy
	// client-side, so nothing internal should ever be fetched here.
	if err := api.ValidateWebhookURL(raw); err != nil {
		api.Error(w, http.StatusBadRequest, "url is not allowed")
		return
	}

	// Optional resize: ?w=<max width px>&q=<jpeg quality 1-100>. When w
	// is set we downscale + re-encode to JPEG (see resizeJPEG). Cards
	// display images far smaller than their source (a 1280px / multi-MB
	// news photo in a ~340-800px column), so this is the dominant
	// payload win. NOTE: `w` here is the http.ResponseWriter — use maxW.
	maxW := 0
	if s := r.URL.Query().Get("w"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxW = n
			if maxW > 2048 {
				maxW = 2048 // sanity cap
			}
		}
	}
	quality := 80
	if s := r.URL.Query().Get("q"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 100 {
			quality = n
		}
	}

	// Cache key is a hash of the full URL PLUS the resize params, so
	// each variant (w/q) caches independently — without this, the first
	// requested size would be served for all sizes. Content-type cached
	// alongside so we serve the right MIME on hit.
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|w=%d|q=%d", raw, maxW, quality)))
	cacheKey := "img:" + hex.EncodeToString(sum[:])
	ctKey := cacheKey + ":ct"

	if h.cache != nil {
		if data, _ := h.cache.Get(r.Context(), cacheKey); len(data) > 0 {
			ct := "image/jpeg"
			if cached, _ := h.cache.Get(r.Context(), ctKey); len(cached) > 0 {
				ct = string(cached)
			}
			w.Header().Set("Content-Type", ct)
			// Long edge cache: external image URLs almost never
			// change content. 7 days at the CDN, 1 day immutable
			// in browser. If a publisher swaps the underlying
			// image we serve stale until cache expires — fine
			// for news photos.
			w.Header().Set("Cache-Control", "public, max-age=86400, s-maxage=604800, immutable")
			w.Header().Set("X-Cache", "HIT")
			_, _ = w.Write(data)
			return
		}
	}

	// Cache miss — fetch from origin.
	req, _ := http.NewRequestWithContext(r.Context(), "GET", raw, nil)
	req.Header.Set("User-Agent", "loomfeed-img-proxy/1.0 (+https://www.loomfeed.com)")
	req.Header.Set("Accept", "image/avif,image/webp,image/jpeg,image/png,image/*;q=0.8,*/*;q=0.5")
	resp, err := h.client.Do(req)
	if err != nil {
		api.Error(w, http.StatusBadGateway, "fetch failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		api.Error(w, http.StatusBadGateway, "origin returned non-200")
		return
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		api.Error(w, http.StatusBadGateway, "origin returned non-image content")
		return
	}

	// Cap downloaded size at 10MB. Read ONE extra byte so we can tell a
	// complete file from a truncated one: caching a silently-truncated
	// image would serve a permanently-broken picture under the immutable
	// cache key, so reject oversize instead.
	const maxImg = 10 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImg+1))
	if err != nil {
		api.Error(w, http.StatusBadGateway, "read failed")
		return
	}
	if len(body) > maxImg {
		api.Error(w, http.StatusBadGateway, "image too large")
		return
	}

	// Downscale + re-encode when a width was requested. Falls back to
	// the original bytes for anything that can't/shouldn't be resized.
	// We cache (and serve) the resized variant under this w/q key.
	if maxW > 0 {
		body, ct, _ = resizeJPEG(body, ct, maxW, quality)
	}

	h.cacheImage(cacheKey, ctKey, body, ct)

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400, s-maxage=604800, immutable")
	w.Header().Set("X-Cache", "MISS")
	_, _ = w.Write(body)
}
