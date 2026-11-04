package bundle

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Redaction modes (docs/privacy-and-redaction.md states the fidelity cost of each).
const (
	Keep          = "keep"           // store MCVs and histogram bounds as they are
	Hash          = "hash"           // replace MCV values with same-type surrogates; drop histogram
	DropMCV       = "drop-mcv"       // remove MCV values and frequencies
	DropHistogram = "drop-histogram" // remove histogram bounds
)

// NewSalt returns a fresh random salt for one export. It is never written to the bundle.
// Because the salt is discarded, surrogates cannot be matched against a dictionary of values later.
func NewSalt() ([]byte, error) {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// Redact applies a mode to one attribute in place. mode must be one of the constants above.
// salt is used only by Hash mode.
func Redact(a *Attribute, mode string, salt []byte) error {
	switch mode {
	case Keep:
		return nil
	case DropMCV:
		a.MCV, a.MCVFreqs = nil, nil
	case DropHistogram:
		a.Histogram = nil
	case Hash:
		return redactHash(a, salt)
	default:
		return fmt.Errorf("unknown redaction mode %q", mode)
	}
	return nil
}

// redactHash replaces MCV values with surrogates of the same type and drops the histogram.
// Columns whose type has no surrogate fall back to DropMCV, and the fallback is recorded in the result.
func redactHash(a *Attribute, salt []byte) error {
	a.Histogram = nil
	if a.MCV == nil {
		return nil
	}
	vals, err := ParseArray(*a.MCV)
	if err != nil {
		return err
	}
	var freqs []string
	if a.MCVFreqs != nil {
		freqs, err = ParseArray(*a.MCVFreqs)
		if err != nil {
			return err
		}
		if len(freqs) != len(vals) {
			return errors.New("most_common_vals and most_common_freqs have different lengths")
		}
	}
	seen := map[string]bool{}
	var outVals, outFreqs []string
	for i, v := range vals {
		s, ok := surrogate(a.DataType, v, salt)
		if !ok {
			// No surrogate for this type: drop the MCV list rather than leak values.
			a.MCV, a.MCVFreqs = nil, nil
			return nil
		}
		if seen[s] {
			continue // collision: keep the first, drop the rest
		}
		seen[s] = true
		outVals = append(outVals, s)
		if freqs != nil {
			outFreqs = append(outFreqs, freqs[i])
		}
	}
	mcv := FormatArray(outVals)
	a.MCV = &mcv
	if freqs != nil {
		f := FormatArray(outFreqs)
		a.MCVFreqs = &f
	}
	return nil
}

// surrogate maps one value to a same-type replacement, or reports that the type is unsupported.
func surrogate(dataType, value string, salt []byte) (string, bool) {
	mac := hmac.New(sha256.New, salt)
	mac.Write([]byte(value))
	sum := mac.Sum(nil)
	switch {
	case isInteger(dataType):
		n := binary.BigEndian.Uint32(sum[:4]) % 2147483647
		return fmt.Sprint(n), true
	case isText(dataType):
		return "h" + hex.EncodeToString(sum[:6]), true
	}
	return "", false
}

func isInteger(t string) bool {
	switch strings.ToLower(t) {
	case "smallint", "integer", "int", "int4", "int2", "bigint", "int8":
		return true
	}
	return false
}

func isText(t string) bool {
	lt := strings.ToLower(t)
	return lt == "text" || strings.HasPrefix(lt, "character varying") || strings.HasPrefix(lt, "varchar") || lt == "name"
}

// ParseArray reads a PostgreSQL one-dimensional array literal such as {a,"b c",NULL}.
// Quoted elements may contain commas, spaces and backslash escapes. The unquoted NULL is returned as "NULL".
func ParseArray(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != '{' || s[len(s)-1] != '}' {
		return nil, fmt.Errorf("not an array literal: %.30q", s)
	}
	body := s[1 : len(s)-1]
	if strings.TrimSpace(body) == "" {
		return nil, nil
	}
	var out []string
	var cur strings.Builder
	quoted, escaped := false, false
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case escaped:
			cur.WriteByte(c)
			escaped = false
		case c == '\\':
			escaped = true
		case c == '"':
			quoted = !quoted
		case c == ',' && !quoted:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	if quoted || escaped {
		return nil, fmt.Errorf("unterminated quote in array literal")
	}
	out = append(out, cur.String())
	return out, nil
}

// FormatArray writes a one-dimensional array literal, quoting every element.
func FormatArray(elems []string) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, e := range elems {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		for _, c := range []byte(e) {
			if c == '"' || c == '\\' {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
		}
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}
