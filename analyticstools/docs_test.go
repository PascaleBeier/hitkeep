package analyticstools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDocsClientBlocksOtherOriginsAndCachesMarkdown(t *testing.T) {
	requests := 0
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if !strings.Contains(r.Header.Get("Accept"), "text/markdown") {
			t.Errorf("expected Accept header to include text/markdown, got %q", r.Header.Get("Accept"))
		}
		if r.URL.Path != "/guides/integrations/mcp/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# MCP Integration\n\nUse the official server.\n"))
	}))
	defer docsTS.Close()

	client := NewDocs(docsTS.URL, time.Hour)
	for i := range 2 {
		page, err := client.GetMarkdown(context.Background(), "/guides/integrations/mcp")
		if err != nil {
			t.Fatalf("GetMarkdown attempt %d: %v", i+1, err)
		}
		if page.Path != "/guides/integrations/mcp/" || !strings.Contains(page.Markdown, "# MCP Integration") {
			t.Fatalf("unexpected page: %+v", page)
		}
	}
	if requests != 1 {
		t.Fatalf("expected cached second docs fetch, got %d requests", requests)
	}
	if _, err := client.GetMarkdown(context.Background(), "https://example.com/guides/integrations/mcp/"); err == nil {
		t.Fatalf("expected other docs origin to be rejected")
	}
	if _, err := client.GetMarkdown(context.Background(), "/guides/%2e%2e/secret"); err == nil {
		t.Fatalf("expected encoded parent traversal to be rejected")
	}
}

func TestDocsClientCoalescesConcurrentFetches(t *testing.T) {
	var requests atomic.Int32
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# MCP Integration\n"))
	}))
	defer docsTS.Close()

	client := NewDocs(docsTS.URL, time.Hour)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			_, err := client.GetMarkdown(context.Background(), "/guides/integrations/mcp/")
			errs <- err
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("GetMarkdown: %v", err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected concurrent requests to coalesce to one fetch, got %d", got)
	}
}

func TestDocsClientCapsCachedPages(t *testing.T) {
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# " + r.URL.Path + "\n"))
	}))
	defer docsTS.Close()

	client := NewDocs(docsTS.URL, time.Hour)
	for i := range maxDocCacheEntries + 5 {
		if _, err := client.GetMarkdown(context.Background(), "/guides/page-"+strconv.Itoa(i)+"/"); err != nil {
			t.Fatalf("GetMarkdown page %d: %v", i, err)
		}
	}

	if got := client.pages.Len(); got != maxDocCacheEntries {
		t.Fatalf("expected docs cache to cap at %d entries, got %d", maxDocCacheEntries, got)
	}
}
