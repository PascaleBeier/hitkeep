// Package jsonapi defines HitKeep's JSON contract exclusively on top of Go's
// v2 JSON implementation.
//
// Decoding uses the secure v2 defaults: object names are case-sensitive,
// duplicate object names are rejected, and invalid UTF-8 is rejected. Unknown
// object members remain forward-compatible unless a caller explicitly requests
// strict schema matching.
//
// Encoding uses v2 semantics plus explicit product-level choices for nil
// collections, map ordering, and HTML/JavaScript-safe strings. Raw JSON
// embedded in an output must satisfy the v2 syntax rules so ambiguous or
// invalid data cannot cross a HitKeep boundary.
package jsonapi

import (
	"bufio"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"io"
)

// Options configures semantic or syntactic JSON behavior. Options from
// encoding/json/v2 and encoding/json/jsontext are assignment-compatible.
type Options = jsonv2.Options

// RawMessage is an independently validated raw JSON value.
type RawMessage = jsontext.Value

var marshalOptions = jsonv2.JoinOptions(
	jsonv2.DefaultOptionsV2(),
	jsonv2.Deterministic(true),
	jsonv2.FormatNilMapAsNull(true),
	jsonv2.FormatNilSliceAsNull(true),
	jsontext.EscapeForHTML(true),
	jsontext.EscapeForJS(true),
)

// Marshal serializes in with HitKeep's v2 wire policy and strict raw-value
// validation. Later options override the shared policy.
func Marshal(in any, opts ...Options) ([]byte, error) {
	return jsonv2.Marshal(in, appendMarshalOptions(opts)...)
}

// MarshalWrite serializes one JSON value directly to out. Unlike the legacy
// Encoder.Encode method, it does not append a newline.
func MarshalWrite(out io.Writer, in any, opts ...Options) error {
	return jsonv2.MarshalWrite(out, in, appendMarshalOptions(opts)...)
}

// MarshalEncode serializes the next value through a jsontext state machine.
// Use it for streams of multiple top-level values such as NDJSON.
func MarshalEncode(out *jsontext.Encoder, in any, opts ...Options) error {
	return jsonv2.MarshalEncode(out, in, appendMarshalOptions(opts)...)
}

// MarshalIndent serializes in using the requested indentation while retaining
// HitKeep's v2 wire policy.
func MarshalIndent(in any, prefix, indent string) ([]byte, error) {
	return Marshal(in, jsontext.WithIndentPrefix(prefix), jsontext.WithIndent(indent))
}

// Unmarshal deserializes exactly one JSON value using the secure v2 defaults.
func Unmarshal(in []byte, out any, opts ...Options) error {
	return jsonv2.Unmarshal(in, out, opts...)
}

// UnmarshalRead deserializes exactly one JSON value and consumes the reader
// through EOF. Trailing non-whitespace data is rejected.
func UnmarshalRead(in io.Reader, out any, opts ...Options) error {
	return jsonv2.UnmarshalRead(in, out, opts...)
}

// UnmarshalReadOptional is UnmarshalRead with an explicit empty-body result.
// It returns io.EOF for an empty or whitespace-only reader while retaining the
// single-value and trailing-data checks for non-empty input.
func UnmarshalReadOptional(in io.Reader, out any, opts ...Options) error {
	reader := bufio.NewReader(in)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return err
		}
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		}
		if err := reader.UnreadByte(); err != nil {
			return err
		}
		return jsonv2.UnmarshalRead(reader, out, opts...)
	}
}

// UnmarshalDecode deserializes the next value from a jsontext state machine.
// Use it for streams of multiple top-level values.
func UnmarshalDecode(in *jsontext.Decoder, out any, opts ...Options) error {
	return jsonv2.UnmarshalDecode(in, out, opts...)
}

// UnmarshalStrict deserializes exactly one value and rejects unknown object
// members in addition to the secure v2 syntax defaults.
func UnmarshalStrict(in []byte, out any, opts ...Options) error {
	opts = append(opts, jsonv2.RejectUnknownMembers(true))
	return jsonv2.Unmarshal(in, out, opts...)
}

// UnmarshalReadStrict is UnmarshalStrict for an io.Reader.
func UnmarshalReadStrict(in io.Reader, out any, opts ...Options) error {
	opts = append(opts, jsonv2.RejectUnknownMembers(true))
	return jsonv2.UnmarshalRead(in, out, opts...)
}

// UnmarshalReadOptionalStrict combines the explicit empty-body behavior of
// UnmarshalReadOptional with unknown-member rejection.
func UnmarshalReadOptionalStrict(in io.Reader, out any, opts ...Options) error {
	opts = append(opts, jsonv2.RejectUnknownMembers(true))
	return UnmarshalReadOptional(in, out, opts...)
}

// Valid reports whether in is one unambiguous RFC 7493 JSON value. In
// particular, duplicate object names and invalid UTF-8 are invalid.
func Valid(in []byte) bool {
	return jsontext.Value(in).IsValid()
}

func appendMarshalOptions(opts []Options) []Options {
	all := make([]Options, 1, len(opts)+1)
	all[0] = marshalOptions
	return append(all, opts...)
}
