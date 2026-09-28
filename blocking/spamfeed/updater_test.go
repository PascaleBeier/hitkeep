package spamfeed

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type trackedReadCloser struct {
	reader io.Reader
	closed bool
}

func (r *trackedReadCloser) Read(p []byte) (int, error) { return r.reader.Read(p) }
func (r *trackedReadCloser) Close() error               { r.closed = true; return nil }

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type singleResponseClient struct{ response *http.Response }

func (c singleResponseClient) Do(*http.Request) (*http.Response, error) { return c.response, nil }

type fakeHTTPClient struct{ responses map[string]string }

func (f fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	body, ok := f.responses[req.URL.String()]
	if !ok {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("missing"))}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
}
func spamhausJSONFeed(cidr string, timestamp int64) string {
	return fmt.Sprintf("{\"cidr\":%q}\n{\"type\":\"metadata\",\"timestamp\":%d,\"records\":1,\"copyright\":\"(c) 2026 The Spamhaus Project SLU\",\"terms\":\"https://www.spamhaus.org/drop/terms/\"}\n", cidr, timestamp)
}

func TestFetchURLClosesResponseBodiesAndRejectsOversize(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		reader     io.Reader
		wantErr    string
		wantBody   string
	}{
		{name: "success", statusCode: http.StatusOK, reader: strings.NewReader("ok"), wantBody: "ok"},
		{name: "status failure", statusCode: http.StatusBadGateway, reader: strings.NewReader("failure"), wantErr: "unexpected HTTP status"},
		{name: "read failure", statusCode: http.StatusOK, reader: failingReader{}, wantErr: "unexpected EOF"},
		{name: "oversize", statusCode: http.StatusOK, reader: strings.NewReader(strings.Repeat("x", maxFeedResponseBytes+1)), wantErr: "response body exceeds"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedReadCloser{reader: tc.reader}
			got, err := fetchURL(context.Background(), singleResponseClient{response: &http.Response{StatusCode: tc.statusCode, Body: body}}, "https://example.com/feed")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("fetch URL: %v", err)
				}
				raw, err := io.ReadAll(got)
				if err != nil {
					t.Fatalf("read bounded response: %v", err)
				}
				if string(raw) != tc.wantBody {
					t.Fatalf("expected body %q, got %q", tc.wantBody, raw)
				}
			}
			if !body.closed {
				t.Fatal("expected original response body to close")
			}
		})
	}

	exact := &trackedReadCloser{reader: strings.NewReader(strings.Repeat("x", maxFeedResponseBytes))}
	got, err := fetchURL(context.Background(), singleResponseClient{response: &http.Response{StatusCode: http.StatusOK, Body: exact}}, "https://example.com/feed")
	if err != nil {
		t.Fatalf("fetch exactly capped response: %v", err)
	}
	raw, err := io.ReadAll(got)
	if err != nil || len(raw) != maxFeedResponseBytes {
		t.Fatalf("expected exactly %d readable bytes, got %d, %v", maxFeedResponseBytes, len(raw), err)
	}
	if !exact.closed {
		t.Fatal("expected exact-cap response body to close")
	}
}

func TestFetchSpamhausCIDRsClosesBodyAfterParserFailure(t *testing.T) {
	body := &trackedReadCloser{reader: strings.NewReader(`{"cidr":`)}
	_, err := fetchSpamhausCIDRs(context.Background(), singleResponseClient{response: &http.Response{StatusCode: http.StatusOK, Body: body}}, "https://example.com/feed", 32)
	if err == nil || !strings.Contains(err.Error(), "decode spamhaus JSON") {
		t.Fatalf("expected parser failure, got %v", err)
	}
	if !body.closed {
		t.Fatal("expected parser failure response body to close")
	}
}

func TestFetchSpamhausCIDRsRejectsInvalidJSONFeeds(t *testing.T) {
	metadata := `{"type":"metadata","timestamp":1784054642,"records":1,"copyright":"(c) 2026 The Spamhaus Project SLU","terms":"https://www.spamhaus.org/drop/terms/"}` + "\n"
	tests := []struct {
		name           string
		body           string
		expectedBitLen int
		wantErr        string
	}{
		{
			name:           "malformed JSON",
			body:           `{"cidr":`,
			expectedBitLen: 32,
			wantErr:        "decode spamhaus JSON",
		},
		{
			name:           "duplicate object name",
			body:           `{"cidr":"203.0.113.0/24","cidr":"198.51.100.0/24"}` + "\n" + metadata,
			expectedBitLen: 32,
			wantErr:        "decode spamhaus JSON",
		},
		{
			name:           "invalid UTF-8",
			body:           string(append([]byte(`{"cidr":"`), append([]byte{0xff}, []byte(`"}`)...)...)),
			expectedBitLen: 32,
			wantErr:        "decode spamhaus JSON",
		},
		{
			name:           "invalid CIDR",
			body:           `{"cidr":"invalid"}` + "\n" + metadata,
			expectedBitLen: 32,
			wantErr:        "invalid cidr",
		},
		{
			name:           "wrong address family",
			body:           spamhausJSONFeed("2001:db8::/32", 1_784_054_642),
			expectedBitLen: 32,
			wantErr:        "IPv6 cidr",
		},
		{
			name:           "missing metadata",
			body:           `{"cidr":"203.0.113.0/24"}` + "\n",
			expectedBitLen: 32,
			wantErr:        "missing terminal metadata",
		},
		{
			name: "duplicate metadata",
			body: spamhausJSONFeed("203.0.113.0/24", 1_784_054_642) +
				`{"type":"metadata","timestamp":1784054643,"records":1,"copyright":"(c) 2026 The Spamhaus Project SLU","terms":"https://www.spamhaus.org/drop/terms/"}` + "\n",
			expectedBitLen: 32,
			wantErr:        "duplicate metadata",
		},
		{
			name: "metadata not terminal",
			body: metadata +
				`{"cidr":"203.0.113.0/24"}` + "\n",
			expectedBitLen: 32,
			wantErr:        "must be terminal",
		},
		{
			name: "record count mismatch",
			body: `{"cidr":"203.0.113.0/24"}` + "\n" +
				`{"type":"metadata","timestamp":1784054642,"records":2,"copyright":"(c) 2026 The Spamhaus Project SLU","terms":"https://www.spamhaus.org/drop/terms/"}` + "\n",
			expectedBitLen: 32,
			wantErr:        "declares 2 records but contains 1",
		},
		{
			name: "missing attribution",
			body: `{"cidr":"203.0.113.0/24"}` + "\n" +
				`{"type":"metadata","timestamp":1784054642,"records":1}` + "\n",
			expectedBitLen: 32,
			wantErr:        "missing copyright",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := fakeHTTPClient{responses: map[string]string{spamhausDropURL: tc.body}}
			_, err := fetchSpamhausCIDRs(context.Background(), client, spamhausDropURL, tc.expectedBitLen)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
