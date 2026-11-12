package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Agents read lacarta's output, and the prototype's hashes are stored in existing state
// files, so a few values are formatted exactly as Python formats them.

// MarshalIndent is json.dumps(v, indent=2): two-space indent, ASCII only, no HTML escaping.
func MarshalIndent(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return asciiJSON(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

// asciiJSON escapes every non-ASCII rune as \uXXXX (surrogate pairs above the BMP), as
// Python's ensure_ascii does. Non-ASCII bytes only occur inside JSON strings.
func asciiJSON(b []byte) []byte {
	var out bytes.Buffer
	for _, r := range string(b) {
		switch {
		case r < 0x80:
			out.WriteRune(r)
		case r > 0xFFFF:
			a, z := utf16.EncodeRune(r)
			fmt.Fprintf(&out, `\u%04x\u%04x`, a, z)
		default:
			fmt.Fprintf(&out, `\u%04x`, r)
		}
	}
	return out.Bytes()
}

// pyDumpsCompact is json.dumps(v) with default separators (", " and ": "), for hashing.
func pyDumpsCompact(v any) string {
	b, _ := json.Marshal(v)
	// Re-encode with Python's separators: walk tokens outside strings.
	var out strings.Builder
	inStr, esc := false, false
	for _, c := range string(asciiJSON(unescapeHTML(b))) {
		if inStr {
			out.WriteRune(c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
			out.WriteRune(c)
		case ',':
			out.WriteString(", ")
		case ':':
			out.WriteString(": ")
		default:
			out.WriteRune(c)
		}
	}
	return out.String()
}

func unescapeHTML(b []byte) []byte {
	r := strings.NewReplacer(`<`, "<", `>`, ">", `&`, "&")
	return []byte(r.Replace(string(b)))
}

// PyStrList is repr() of a list of strings: ['a', "it's"].
func PyStrList(xs []string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = pyStrRepr(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func pyStrRepr(s string) string {
	q := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = `"`
	}
	var b strings.Builder
	b.WriteString(q)
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case string(r) == q:
			b.WriteString(`\` + q)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteString(q)
	return b.String()
}

// PyIntList is repr() of a list of ints: [3, 5].
func PyIntList(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func lower(s string) string               { return strings.ToLower(s) }
func join(xs []string, sep string) string { return strings.Join(xs, sep) }

// Capitalize is str.capitalize(): first letter upper, the rest lower.
func Capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

// PyDumps is json.dumps(v): one line, ", " and ": " separators, ASCII only.
func PyDumps(v any) string { return pyDumpsCompact(v) }

// PyDumpsValue is PyDumps for values that may fail to encode.
func PyDumpsValue(v any) (string, error) {
	if _, err := json.Marshal(v); err != nil {
		return "", err
	}
	return pyDumpsCompact(v), nil
}
