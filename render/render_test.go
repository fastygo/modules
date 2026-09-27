package render

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/a-h/templ"

	"github.com/fastygo/framework/pkg/cache"
)

func TestRenderWritesHTML(t *testing.T) {
	rec := httptest.NewRecorder()
	component := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<p>ok</p>")
		return err
	})
	if err := Render(context.Background(), rec, component); err != nil {
		t.Fatal(err)
	}
	if rec.Body.String() != "<p>ok</p>" {
		t.Fatalf("body %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content type %q", ct)
	}
}

func TestCachedRenderReusesBytes(t *testing.T) {
	htmlCache := cache.New[[]byte](time.Minute)
	component := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<p>cached</p>")
		return err
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := CachedRender(context.Background(), rec, req, htmlCache, "k", component); err != nil {
		t.Fatal(err)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag")
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-None-Match", etag)
	if err := CachedRender(context.Background(), rec2, req2, htmlCache, "k", component); err != nil {
		t.Fatal(err)
	}
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("status %d", rec2.Code)
	}
}
