// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package jcs implements the JSON Canonicalization Scheme (RFC 8785) subset
// required by COORD Specification 1.0, section 5.2: object keys sorted by
// UTF-16 code units, no insignificant whitespace, minimal string escaping,
// and integer-only numbers. A data item that cannot be serialized canonically
// is an error, never a non-canonical output.
package jcs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// ErrNombreNonCanonique signals a non-integer number: COORD data uses
// integers only; anything else MUST be refused, not approximated.
var ErrNombreNonCanonique = errors.New("nombre non canonique (entier attendu)")

// Canonical returns the RFC 8785 canonical serialization of v (JSON-decoded
// values: nil, bool, float64 integers, string, []any, map[string]any).
func Canonical(v any) ([]byte, error) {
	var buf []byte
	b, err := encode(v, buf)
	return b, err
}

// Fingerprint returns the SHA-256 hex fingerprint (COORD spec 5.3) of the
// canonical form of v.
func Fingerprint(v any) (string, error) {
	b, err := Canonical(v)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

func encode(v any, out []byte) ([]byte, error) {
	switch x := v.(type) {
	case nil:
		return append(out, "null"...), nil
	case bool:
		if x {
			return append(out, "true"...), nil
		}
		return append(out, "false"...), nil
	case string:
		return encodeString(x, out), nil
	case int:
		return append(out, strconv.Itoa(x)...), nil
	case int64:
		return append(out, strconv.FormatInt(x, 10)...), nil
	case float64:
		if !isInteger(x) {
			return out, fmt.Errorf("%w: %v", ErrNombreNonCanonique, x)
		}
		return append(out, strconv.FormatInt(int64(x), 10)...), nil
	case []any:
		out = append(out, '[')
		for i, e := range x {
			if i > 0 {
				out = append(out, ',')
			}
			var err error
			if out, err = encode(e, out); err != nil {
				return out, err
			}
		}
		return append(out, ']'), nil
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.SliceStable(keys, func(i, j int) bool { return utf16Less(keys[i], keys[j]) })
		out = append(out, '{')
		for i, k := range keys {
			if i > 0 {
				out = append(out, ',')
			}
			out = encodeString(k, out)
			out = append(out, ':')
			var err error
			if out, err = encode(x[k], out); err != nil {
				return out, err
			}
		}
		return append(out, '}'), nil
	default:
		return out, fmt.Errorf("type non canonique: %T", v)
	}
}

func isInteger(f float64) bool {
	return !math.IsInf(f, 0) && !math.IsNaN(f) && math.Trunc(f) == f &&
		math.Abs(f) <= 9.007199254740992e15 // entier sure en float64
}

// utf16Less compares two strings by UTF-16 code units (RFC 8785, 3.2.3).
func utf16Less(a, b string) bool {
	ua := utf16.Encode([]rune(a))
	ub := utf16.Encode([]rune(b))
	n := len(ua)
	if len(ub) < n {
		n = len(ub)
	}
	for i := 0; i < n; i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

// encodeString applies minimal escaping (RFC 8785, 3.2.2.2).
func encodeString(s string, out []byte) []byte {
	out = append(out, '"')
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			switch c {
			case '"':
				out = append(out, '\\', '"')
			case '\\':
				out = append(out, '\\', '\\')
			case '\b':
				out = append(out, '\\', 'b')
			case '\f':
				out = append(out, '\\', 'f')
			case '\n':
				out = append(out, '\\', 'n')
			case '\r':
				out = append(out, '\\', 'r')
			case '\t':
				out = append(out, '\\', 't')
			default:
				if c < 0x20 {
					out = append(out, '\\', 'u', '0', '0',
						hexDigit(c>>4), hexDigit(c&0xF))
				} else {
					out = append(out, c)
				}
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		out = append(out, s[i:i+size]...)
		_ = r
		i += size
	}
	return append(out, '"')
}

func hexDigit(b byte) byte {
	if b < 10 {
		return '0' + b
	}
	return 'a' + b - 10
}
