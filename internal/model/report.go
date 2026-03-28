package model

import (
	"crypto/rand"
	"fmt"
	"time"
)

// Status represents the lifecycle state of a report.
type Status string

const (
	StatusQueued     Status = "queued"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// Report holds persistent metadata for a single uploaded report.
type Report struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Shard returns the two-character hex prefix used for directory sharding.
func (r *Report) Shard() string {
	if len(r.ID) < 2 {
		return "00"
	}
	return r.ID[:2]
}

// NewID generates a random UUID v4 string using crypto/rand.
func NewID() (string, error) {
	var randBytes [16]byte
	if _, err := rand.Read(randBytes[:]); err != nil {
		return "", err
	}
	// UUID v4 requires two specific bit fields to be fixed, regardless of the random content:
	// byte 6: top 4 bits must be 0100 (version 4) — mask clears them, OR sets them.
	// byte 8: top 2 bits must be 10 (RFC 4122 variant) — same pattern.
	randBytes[6] = (randBytes[6] & 0x0f) | 0x40
	randBytes[8] = (randBytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		randBytes[0:4], randBytes[4:6], randBytes[6:8], randBytes[8:10], randBytes[10:16]), nil
}
