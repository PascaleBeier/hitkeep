package jsonapi

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestUnmarshalUsesSecureV2Defaults(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "duplicate object name", raw: []byte(`{"name":"first","name":"second"}`)},
		{name: "invalid UTF-8", raw: []byte{'{', '"', 'n', 'a', 'm', 'e', '"', ':', '"', 0xff, '"', '}'}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out map[string]string
			if err := Unmarshal(tt.raw, &out); err == nil {
				t.Fatalf("Unmarshal(%q) unexpectedly succeeded: %#v", tt.raw, out)
			}
			if Valid(tt.raw) {
				t.Fatalf("Valid(%q) = true, want false", tt.raw)
			}
		})
	}
}

func TestUnmarshalUsesCaseSensitiveNames(t *testing.T) {
	var out struct {
		Name string `json:"name"`
	}
	if err := Unmarshal([]byte(`{"Name":"wrong","name":"right"}`), &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Name != "right" {
		t.Fatalf("Name = %q, want right", out.Name)
	}
}

func TestUnmarshalIgnoresUnknownMembersByDefault(t *testing.T) {
	var out struct {
		Name string `json:"name"`
	}
	if err := Unmarshal([]byte(`{"name":"kept","future_field":true}`), &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Name != "kept" {
		t.Fatalf("Name = %q, want kept", out.Name)
	}
}

func TestStrictUnmarshalRejectsUnknownMembers(t *testing.T) {
	var out struct {
		Name string `json:"name"`
	}
	err := UnmarshalStrict([]byte(`{"name":"ok","extra":true}`), &out)
	if err == nil {
		t.Fatal("UnmarshalStrict unexpectedly accepted an unknown member")
	}
}

func TestUnmarshalReadRequiresExactlyOneValue(t *testing.T) {
	var out map[string]bool
	if err := UnmarshalRead(strings.NewReader(`{"ok":true} {"second":true}`), &out); err == nil {
		t.Fatal("UnmarshalRead unexpectedly accepted trailing JSON")
	}
}

func TestUnmarshalReadOptionalReportsEmptyInputAsEOF(t *testing.T) {
	var out any
	for _, input := range []string{"", " \t\r\n"} {
		if err := UnmarshalReadOptional(strings.NewReader(input), &out); !errors.Is(err, io.EOF) {
			t.Fatalf("UnmarshalReadOptional(%q) error = %v, want EOF", input, err)
		}
	}
}

func TestUnmarshalReadOptionalStillRejectsTrailingJSON(t *testing.T) {
	var out map[string]bool
	if err := UnmarshalReadOptional(strings.NewReader(`{"ok":true} {"second":true}`), &out); err == nil {
		t.Fatal("UnmarshalReadOptional unexpectedly accepted trailing JSON")
	}
}

func TestStreamingDecoderUsesJSONTextStateMachine(t *testing.T) {
	decoder := jsontext.NewDecoder(strings.NewReader("{\"n\":1}\n{\"n\":2}\n"))
	for want := 1; want <= 2; want++ {
		var out struct {
			N int `json:"n"`
		}
		if err := UnmarshalDecode(decoder, &out); err != nil {
			t.Fatalf("Decode %d: %v", want, err)
		}
		if out.N != want {
			t.Fatalf("Decode %d produced %d", want, out.N)
		}
	}
	var trailing any
	if err := UnmarshalDecode(decoder, &trailing); !errors.Is(err, io.EOF) {
		t.Fatalf("trailing Decode error = %v, want EOF", err)
	}
}

func TestMarshalUsesExplicitV2WirePolicy(t *testing.T) {
	type payload struct {
		Empty      int               `json:"empty,omitempty"`
		Enabled    bool              `json:"enabled,omitempty"`
		Items      []string          `json:"items"`
		Meta       map[string]string `json:"meta"`
		Ordered    map[string]int    `json:"ordered"`
		HTML       string            `json:"html"`
		JavaScript string            `json:"javascript"`
	}

	raw, err := Marshal(payload{
		Ordered:    map[string]int{"z": 2, "a": 1},
		HTML:       "<script>&",
		JavaScript: "line\u2028separator",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"empty":0,"enabled":false,"items":null,"meta":null,"ordered":{"a":1,"z":2},"html":"\u003cscript\u003e\u0026","javascript":"line\u2028separator"}`
	if string(raw) != want {
		t.Fatalf("Marshal = %s, want %s", raw, want)
	}
}

func TestMarshalRejectsAmbiguousRawJSON(t *testing.T) {
	_, err := Marshal(struct {
		Raw RawMessage `json:"raw"`
	}{Raw: RawMessage(`{"id":1,"id":2}`)})
	if err == nil {
		t.Fatal("Marshal unexpectedly accepted duplicate names in a raw value")
	}
}

func TestMarshalIndentAndEncoderOptions(t *testing.T) {
	indented, err := MarshalIndent(map[string]int{"value": 1}, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if !bytes.Contains(indented, []byte("\n  \"value\"")) {
		t.Fatalf("MarshalIndent output is not indented: %q", indented)
	}

	var out bytes.Buffer
	encoder := jsontext.NewEncoder(&out)
	if err := MarshalEncode(encoder, "<value>", jsontext.EscapeForHTML(false)); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got, want := out.String(), "\"<value>\"\n"; got != want {
		t.Fatalf("Encode = %q, want %q", got, want)
	}
}

func TestRawMessageUsesJSONTextValidation(t *testing.T) {
	raw := RawMessage(`{"ok":true}`)
	if !raw.IsValid() {
		t.Fatal("RawMessage should be a valid jsontext.Value")
	}
	if raw.Kind() != '{' {
		t.Fatalf("RawMessage kind = %q, want object", raw.Kind())
	}

	formatted := raw.Clone()
	if err := formatted.Format(jsontext.WithIndent("  ")); err != nil {
		t.Fatalf("Format: %v", err)
	}
}
