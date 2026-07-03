package build

import (
    "time"

    "cloudforge/pkg/hash"
)

// StepType constants
const (
    StepAdd = "add"
)

// Step represents a single build instruction (deterministic subset).
type Step struct {
    Type    string `json:"type"`
    Path    string `json:"path"`
    Content []byte `json:"content"`
}

// Result describes a produced layer.
type Result struct {
    ID        hash.Digest
    Size      int64
    MediaType string
    CreatedAt time.Time
}
