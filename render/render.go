// Package render writes templ components to an HTTP response.
// It is module github.com/fastygo/modules/render, so applications that do not
// render templ do not compile github.com/a-h/templ.
package render

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"net/http"

	"github.com/a-h/templ"

	"github.com/fastygo/framework/pkg/cache"
)

// Render executes a templ component into an in-memory buffer, sets the
// HTML content type, and writes the buffer to w. Buffering ensures the
// component finishes successfully before any bytes hit the network — a
// mid-render failure surfaces as a normal Go error instead of a partial
// 200 response.
func Render(ctx context.Context, w http.ResponseWriter, component templ.Component) error {
	buf := &bytes.Buffer{}
	if err := component.Render(ctx, buf); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := w.Write(buf.Bytes())
	return err
}

// CachedRender renders component once per cache key, reusing the bytes on
// every subsequent hit. It also negotiates ETag/If-None-Match so an
// already-cached response can short-circuit to 304 Not Modified.
//
// A nil htmlCache or empty key disables caching and falls through to Render.
func CachedRender(ctx context.Context, w http.ResponseWriter, r *http.Request, htmlCache *cache.Cache[[]byte], key string, component templ.Component) error {
	if htmlCache == nil || key == "" {
		return Render(ctx, w, component)
	}

	if cached, ok := htmlCache.Get(key); ok {
		etag := htmlETag(cached)
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return nil
		}
		_, err := w.Write(cached)
		return err
	}

	buf := &bytes.Buffer{}
	if err := component.Render(ctx, buf); err != nil {
		return err
	}
	body := append([]byte(nil), buf.Bytes()...)
	htmlCache.Set(key, body)

	etag := htmlETag(body)
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	_, err := w.Write(body)
	return err
}

func htmlETag(value []byte) string {
	hasher := fnv.New64a()
	_, _ = hasher.Write(value)
	sum := hasher.Sum64()
	return fmt.Sprintf("\"%x\"", sum)
}
