package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxDocBytes = 2 << 20

type docsClient struct {
	base       *url.URL
	ttl        time.Duration
	httpClient *http.Client

	mu    sync.Mutex
	llms  docCacheEntry
	pages map[string]docCacheEntry
}

type docCacheEntry struct {
	page      docPage
	expiresAt time.Time
}

type docPage struct {
	URL      string
	Path     string
	Markdown string
}

type docSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Path    string `json:"path"`
	Snippet string `json:"snippet"`
	Score   int    `json:"score"`
}

type catalogEntry struct {
	Title       string
	Path        string
	URL         string
	Description string
}

func newDocsClient(baseURL string, ttl time.Duration) *docsClient {
	if ttl <= 0 {
		ttl = time.Hour
	}
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		parsed, _ = url.Parse("https://hitkeep.com")
	}
	return &docsClient{
		base: parsed,
		ttl:  ttl,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		pages: make(map[string]docCacheEntry),
	}
}

func (c *docsClient) Search(ctx context.Context, query string, limit int) ([]docSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	llms, err := c.GetMarkdown(ctx, "/llms.txt")
	if err != nil {
		return nil, err
	}

	terms := searchTerms(query)
	results := make([]docSearchResult, 0)
	for _, entry := range parseCatalog(llms.Markdown, c.base) {
		score := scoreText(terms, entry.Title, entry.Path, entry.Description)
		if score == 0 {
			continue
		}
		results = append(results, docSearchResult{
			Title:   entry.Title,
			URL:     entry.URL,
			Path:    entry.Path,
			Snippet: entry.Description,
			Score:   score,
		})
	}

	c.mu.Lock()
	for _, cached := range c.pages {
		if cached.page.Path == "/llms.txt" {
			continue
		}
		snippet, score := markdownSnippet(terms, cached.page.Markdown)
		if score > 0 {
			results = append(results, docSearchResult{
				Title:   titleFromMarkdown(cached.page.Markdown, cached.page.Path),
				URL:     cached.page.URL,
				Path:    cached.page.Path,
				Snippet: snippet,
				Score:   score,
			})
		}
	}
	c.mu.Unlock()

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Path < results[j].Path
		}
		return results[i].Score > results[j].Score
	})
	return limitSlice(results, limit), nil
}

func (c *docsClient) GetMarkdown(ctx context.Context, rawPath string) (docPage, error) {
	path, err := c.normalizePath(rawPath)
	if err != nil {
		return docPage{}, err
	}

	now := time.Now()
	c.mu.Lock()
	if cached, ok := c.pages[path]; ok && now.Before(cached.expiresAt) {
		c.mu.Unlock()
		return cached.page, nil
	}
	if path == "/llms.txt" && now.Before(c.llms.expiresAt) {
		page := c.llms.page
		c.mu.Unlock()
		return page, nil
	}
	c.mu.Unlock()

	page, err := c.fetchMarkdown(ctx, path)
	if err != nil {
		return docPage{}, err
	}
	entry := docCacheEntry{page: page, expiresAt: now.Add(c.ttl)}
	c.mu.Lock()
	c.pages[path] = entry
	if path == "/llms.txt" {
		c.llms = entry
	}
	c.mu.Unlock()
	return page, nil
}

func (c *docsClient) normalizePath(rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", errors.New("path is required")
	}
	if rawPath == "hitkeep://docs/llms" {
		return "/llms.txt", nil
	}
	if after, ok := strings.CutPrefix(rawPath, "hitkeep://docs/"); ok {
		rawPath = "/" + after
	}

	if parsed, err := url.Parse(rawPath); err == nil && parsed.IsAbs() {
		if parsed.Scheme != c.base.Scheme || parsed.Host != c.base.Host {
			return "", errors.New("docs URL must use the configured HitKeep docs origin")
		}
		rawPath = parsed.EscapedPath()
	}
	if !strings.HasPrefix(rawPath, "/") {
		rawPath = "/" + rawPath
	}
	decodedPath, err := url.PathUnescape(rawPath)
	if err != nil {
		return "", errors.New("docs path must be valid URL path encoding")
	}
	if strings.Contains(decodedPath, "..") {
		return "", errors.New("docs path must not contain '..'")
	}
	if rawPath != "/llms.txt" && !strings.Contains(pathBase(rawPath), ".") && !strings.HasSuffix(rawPath, "/") {
		rawPath += "/"
	}
	return rawPath, nil
}

func (c *docsClient) fetchMarkdown(ctx context.Context, path string) (docPage, error) {
	target := *c.base
	target.Path = path
	target.RawQuery = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return docPage{}, err
	}
	req.Header.Set("Accept", "text/markdown")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return docPage{}, fmt.Errorf("fetch docs markdown: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return docPage{}, fmt.Errorf("fetch docs markdown: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDocBytes+1))
	if err != nil {
		return docPage{}, err
	}
	if len(body) > maxDocBytes {
		return docPage{}, errors.New("docs response too large")
	}
	return docPage{URL: target.String(), Path: path, Markdown: string(body)}, nil
}

func parseCatalog(markdown string, base *url.URL) []catalogEntry {
	lines := strings.Split(markdown, "\n")
	entries := make([]catalogEntry, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		titleEnd := strings.Index(line, "](")
		pathEnd := strings.Index(line, ")")
		if titleEnd < 3 || pathEnd <= titleEnd+2 {
			continue
		}
		title := strings.TrimSpace(line[3:titleEnd])
		path := strings.TrimSpace(line[titleEnd+2 : pathEnd])
		description := strings.TrimSpace(strings.TrimPrefix(line[pathEnd+1:], ":"))
		if title == "" || path == "" {
			continue
		}
		u := base.ResolveReference(&url.URL{Path: path})
		entries = append(entries, catalogEntry{Title: title, Path: path, URL: u.String(), Description: description})
	}
	return entries
}

func searchTerms(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	terms := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, ".,:;!?()[]{}\"'")
		if len(field) < 2 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		terms = append(terms, field)
	}
	return terms
}

func scoreText(terms []string, parts ...string) int {
	if len(terms) == 0 {
		return 0
	}
	text := strings.ToLower(strings.Join(parts, " "))
	score := 0
	for _, term := range terms {
		if strings.Contains(text, term) {
			score++
		}
	}
	return score
}

func markdownSnippet(terms []string, markdown string) (string, int) {
	lines := strings.Split(markdown, "\n")
	best := ""
	bestScore := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		score := scoreText(terms, line)
		if score > bestScore {
			bestScore = score
			best = line
		}
	}
	return best, bestScore
}

func titleFromMarkdown(markdown, fallback string) string {
	for line := range strings.SplitSeq(markdown, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(after)
		}
	}
	return fallback
}

func pathBase(path string) string {
	path = strings.TrimRight(path, "/")
	idx := strings.LastIndex(path, "/")
	if idx >= 0 {
		return path[idx+1:]
	}
	return path
}
