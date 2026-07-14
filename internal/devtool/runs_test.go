package devtool

import (
	"bytes"
	"strings"
	"testing"
)

func TestRedactingWriterHandlesSecretsSplitAcrossWrites(t *testing.T) {
	var output bytes.Buffer
	writer := &redactingWriter{writer: &output}
	for _, chunk := range []string{"\x1b[33mstarting\x1b[0m\nTOKEN=su", "per-secret\nAuthorization: Bear", "er abc.def\ntrailing"} {
		if _, err := writer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	value := output.String()
	if strings.Contains(value, "super-secret") || strings.Contains(value, "abc.def") {
		t.Fatalf("secret leaked through split write: %q", value)
	}
	if strings.Contains(value, "\x1b[") {
		t.Fatalf("ANSI escape leaked through structured log: %q", value)
	}
	if !strings.Contains(value, "TOKEN=[redacted]") || !strings.Contains(value, "Authorization: [redacted]") || !strings.HasSuffix(value, "trailing") {
		t.Fatalf("unexpected redacted output: %q", value)
	}
}
