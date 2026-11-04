package bundle

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func strp(s string) *string { return &s }

func TestParseArrayRoundTrip(t *testing.T) {
	cases := [][]string{
		{"plain", "with space", "comma,inside", `quote"and\slash`, ""},
		{"NULL"},
		{"0.5", "-1"},
	}
	for _, elems := range cases {
		lit := FormatArray(elems)
		got, err := ParseArray(lit)
		if err != nil {
			t.Fatalf("ParseArray(%s): %v", lit, err)
		}
		if strings.Join(got, "\x00") != strings.Join(elems, "\x00") {
			t.Fatalf("round trip changed %v -> %v (literal %s)", elems, got, lit)
		}
	}
}

func TestParseArrayFromPgStatsForm(t *testing.T) {
	got, err := ParseArray(`{open,closed,"returned it"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2] != "returned it" {
		t.Fatalf("got %v", got)
	}
}

func TestParseArrayRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "open", "{unterminated", `{"a}`} {
		if _, err := ParseArray(in); err == nil {
			t.Errorf("ParseArray(%q): expected error", in)
		}
	}
}

func TestDropMCVAndDropHistogram(t *testing.T) {
	a := &Attribute{DataType: "text", MCV: strp(`{open}`), MCVFreqs: strp(`{0.05}`), Histogram: strp(`{a,z}`)}
	if err := Redact(a, DropMCV, nil); err != nil {
		t.Fatal(err)
	}
	if a.MCV != nil || a.MCVFreqs != nil || a.Histogram == nil {
		t.Fatalf("drop-mcv removed the wrong fields: %+v", a)
	}
	b := &Attribute{DataType: "text", Histogram: strp(`{a,z}`)}
	if err := Redact(b, DropHistogram, nil); err != nil {
		t.Fatal(err)
	}
	if b.Histogram != nil {
		t.Fatal("drop-histogram must remove the histogram")
	}
}

func TestHashRemovesLiteralsAndKeepsFrequencies(t *testing.T) {
	salt, _ := NewSalt()
	a := &Attribute{DataType: "text", MCV: strp(`{open,closed,returned}`), MCVFreqs: strp(`{0.05,0.8,0.15}`), Histogram: strp(`{a,z}`)}
	if err := Redact(a, Hash, salt); err != nil {
		t.Fatal(err)
	}
	if a.Histogram != nil {
		t.Fatal("hash mode must drop the histogram (surrogates cannot keep its order)")
	}
	vals, _ := ParseArray(*a.MCV)
	freqs, _ := ParseArray(*a.MCVFreqs)
	if len(vals) != 3 || len(freqs) != 3 {
		t.Fatalf("expected 3 values and 3 frequencies, got %v and %v", vals, freqs)
	}
	for _, v := range vals {
		if strings.Contains(v, "open") || strings.Contains(v, "closed") || strings.Contains(v, "returned") {
			t.Fatalf("surrogate %q still contains a literal", v)
		}
	}
	if freqs[1] != "0.8" {
		t.Fatalf("frequencies must be kept in order, got %v", freqs)
	}
}

func TestHashIntegerSurrogatesAreIntegers(t *testing.T) {
	salt, _ := NewSalt()
	a := &Attribute{DataType: "integer", MCV: strp(`{4242,77}`), MCVFreqs: strp(`{0.01,0.02}`)}
	if err := Redact(a, Hash, salt); err != nil {
		t.Fatal(err)
	}
	vals, _ := ParseArray(*a.MCV)
	for _, v := range vals {
		for _, c := range v {
			if c < '0' || c > '9' {
				t.Fatalf("integer surrogate %q is not an integer", v)
			}
		}
	}
}

func TestHashFallsBackForUnsupportedTypes(t *testing.T) {
	salt, _ := NewSalt()
	a := &Attribute{DataType: "timestamp without time zone", MCV: strp(`{"2026-01-01 00:00:00"}`), MCVFreqs: strp(`{0.5}`)}
	if err := Redact(a, Hash, salt); err != nil {
		t.Fatal(err)
	}
	if a.MCV != nil {
		t.Fatal("unsupported type must drop the MCV list rather than leak values")
	}
}

func TestHashSaltChangesSurrogates(t *testing.T) {
	s1, _ := NewSalt()
	s2, _ := NewSalt()
	a := &Attribute{DataType: "text", MCV: strp(`{open}`), MCVFreqs: strp(`{0.5}`)}
	b := &Attribute{DataType: "text", MCV: strp(`{open}`), MCVFreqs: strp(`{0.5}`)}
	_ = Redact(a, Hash, s1)
	_ = Redact(b, Hash, s2)
	if *a.MCV == *b.MCV {
		t.Fatal("different salts must give different surrogates, so a bundle cannot be matched against a dictionary")
	}
}

func TestKeepLeavesEverything(t *testing.T) {
	a := &Attribute{DataType: "text", MCV: strp(`{open}`), MCVFreqs: strp(`{0.5}`), Histogram: strp(`{a,z}`)}
	before := *a.MCV
	if err := Redact(a, Keep, nil); err != nil {
		t.Fatal(err)
	}
	if *a.MCV != before || a.Histogram == nil {
		t.Fatal("keep must not change the attribute")
	}
}

func TestSignVerify(t *testing.T) {
	dir := t.TempDir()
	priv, pub := filepath.Join(dir, "k.key"), filepath.Join(dir, "k.pub")
	if err := GenerateKeys(priv, pub); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"schema_version":1}`)
	sig, err := Sign(data, priv)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(data, sig, pub); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := Verify([]byte(`{"schema_version":1,"x":1}`), sig, pub); err == nil {
		t.Fatal("a changed bundle must fail verification")
	}
}

func TestBundleRoundTripAndVersionCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.json")
	b := NewBundle(time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC))
	b.Server = ServerInfo{VersionNum: 180006, Version: "18.6"}
	if err := Write(path, b); err != nil {
		t.Fatal(err)
	}
	got, _, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Server.VersionNum != 180006 || got.ExportedAt != "2026-11-04T12:00:00Z" {
		t.Fatalf("round trip lost data: %+v", got)
	}
	b.SchemaVersion = 99
	_ = Write(path, b)
	if _, _, err := Read(path); err == nil {
		t.Fatal("an unsupported schema version must be rejected")
	}
}
