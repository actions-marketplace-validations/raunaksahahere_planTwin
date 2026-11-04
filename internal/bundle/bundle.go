// Package bundle defines the stats bundle: the only artifact that leaves production.
//
// A bundle holds planner statistics (pg_class sizes and pg_stats entries), planner
// settings and the server version. It holds no table rows. Values that can reveal
// data (most common values and histogram bounds) are transformed by a redaction
// mode before the bundle is written (ADR-0011, docs/privacy-and-redaction.md).
package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// SchemaVersion is the bundle format version. Bump it on any incompatible change.
const SchemaVersion = 1

// Bundle is the JSON document written by `plantwin export`.
type Bundle struct {
	SchemaVersion      int               `json:"schema_version"`
	ExportedAt         string            `json:"exported_at"`
	Server             ServerInfo        `json:"server"`
	Settings           map[string]string `json:"settings"`
	Redaction          RedactionInfo     `json:"redaction"`
	Relations          []Relation        `json:"relations"`
	Attributes         []Attribute       `json:"attributes"`
	ExtendedStatistics []string          `json:"extended_statistics"`
	Warnings           []string          `json:"warnings"`
}

// ServerInfo identifies the source server. The twin must use the same major version.
type ServerInfo struct {
	VersionNum int    `json:"version_num"` // for example 180006
	Version    string `json:"version"`     // for example "18.6"
}

// RedactionInfo records which redaction was applied, so the bundle documents itself.
type RedactionInfo struct {
	Default string            `json:"default"`
	Columns map[string]string `json:"columns,omitempty"`
}

// Relation is one table or index: its size statistics.
type Relation struct {
	Schema     string  `json:"schema"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"` // r = table, m = matview, i = index
	RelPages   int32   `json:"relpages"`
	RelTuples  float32 `json:"reltuples"`
	AllVisible int32   `json:"relallvisible"`
	AllFrozen  int32   `json:"relallfrozen"`
}

// Attribute is one column's pg_stats entry. Text fields hold the array literal from pg_stats.
type Attribute struct {
	Schema      string   `json:"schema"`
	Table       string   `json:"table"`
	Column      string   `json:"column"`
	DataType    string   `json:"data_type"`
	Inherited   bool     `json:"inherited"`
	NullFrac    float32  `json:"null_frac"`
	AvgWidth    int32    `json:"avg_width"`
	NDistinct   float32  `json:"n_distinct"`
	MCV         *string  `json:"most_common_vals"`
	MCVFreqs    *string  `json:"most_common_freqs"`
	Histogram   *string  `json:"histogram_bounds"`
	Correlation *float32 `json:"correlation"`
}

// Write encodes the bundle as indented JSON.
func Write(path string, b *Bundle) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Read decodes a bundle and checks the schema version.
func Read(path string) (*Bundle, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, data, fmt.Errorf("bundle is not valid JSON: %w", err)
	}
	if b.SchemaVersion != SchemaVersion {
		return nil, data, fmt.Errorf("bundle schema version %d is not supported (this build reads %d)", b.SchemaVersion, SchemaVersion)
	}
	return &b, data, nil
}

// NewBundle returns an empty bundle stamped with the export time (UTC, RFC 3339).
func NewBundle(now time.Time) *Bundle {
	return &Bundle{
		SchemaVersion: SchemaVersion,
		ExportedAt:    now.UTC().Format(time.RFC3339),
		Settings:      map[string]string{},
	}
}
