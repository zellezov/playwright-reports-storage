package model

import (
	"strings"
	"testing"
)

func TestNewIDFormat(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}

	// UUID format: 8-4-4-4-12 hex digits separated by dashes.
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 dash-separated groups, got %d: %s", len(parts), id)
	}
	expectedLengths := []int{8, 4, 4, 4, 12}
	for i, part := range parts {
		if len(part) != expectedLengths[i] {
			t.Errorf("group %d: want length %d, got %d (%s)", i, expectedLengths[i], len(part), part)
		}
	}
}

func TestNewIDVersion4Bits(t *testing.T) {
	id, _ := NewID()
	parts := strings.Split(id, "-")

	// Third group's first hex digit must be '4' — version 4.
	if parts[2][0] != '4' {
		t.Errorf("expected version digit '4', got '%c' in id %s", parts[2][0], id)
	}

	// Fourth group's first hex digit must be 8, 9, a, or b — RFC 4122 variant bits.
	variantDigit := parts[3][0]
	if variantDigit != '8' && variantDigit != '9' && variantDigit != 'a' && variantDigit != 'b' {
		t.Errorf("expected variant digit in [89ab], got '%c' in id %s", variantDigit, id)
	}
}

func TestNewIDUniqueness(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate ID generated after %d attempts: %s", i, id)
		}
		seen[id] = true
	}
}

func TestShard(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"abcdef12-0000-4000-8000-000000000000", "ab"},
		{"ff000000-0000-4000-8000-000000000000", "ff"},
		{"00112233-0000-4000-8000-000000000000", "00"},
	}
	for _, c := range cases {
		r := &Report{ID: c.id}
		if got := r.Shard(); got != c.want {
			t.Errorf("Shard(%s): want %s, got %s", c.id, c.want, got)
		}
	}
}
