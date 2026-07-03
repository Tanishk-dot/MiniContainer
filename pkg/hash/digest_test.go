package hash

import (
	"strings"
	"testing"
)

func TestFromBytesDeterministic(t *testing.T) {
	d1 := FromBytes([]byte("cloudforge layer content"))
	d2 := FromBytes([]byte("cloudforge layer content"))

	if d1 != d2 {
		t.Fatalf("expected identical digests, got %q and %q", d1, d2)
	}
	if d1.Algorithm != AlgorithmSHA256 {
		t.Fatalf("expected sha256 algorithm, got %q", d1.Algorithm)
	}
	if len(d1.Hex) != HexLength {
		t.Fatalf("expected %d hex chars, got %d", HexLength, len(d1.Hex))
	}
}

func TestFromBytesDifferentContent(t *testing.T) {
	d1 := FromBytes([]byte("layer-a"))
	d2 := FromBytes([]byte("layer-b"))
	if d1 == d2 {
		t.Fatal("expected different digests for different content")
	}
}

func TestParseRoundTrip(t *testing.T) {
	original := FromBytes([]byte("parse me"))
	parsed, err := Parse(original.String())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed != original {
		t.Fatalf("Parse() = %q, want %q", parsed, original)
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []string{
		"not-a-digest",
		"sha256:abc",
		"md5:" + strings.Repeat("a", HexLength),
		"sha256:ghij", // non-hex
	}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Fatalf("Parse(%q) expected error", c)
		}
	}
}

func TestFromReaderMatchesFromBytes(t *testing.T) {
	content := []byte("streaming hash content")
	fromBytes := FromBytes(content)

	fromReader, n, err := FromReader(strings.NewReader(string(content)))
	if err != nil {
		t.Fatalf("FromReader() error = %v", err)
	}
	if n != int64(len(content)) {
		t.Fatalf("FromReader() size = %d, want %d", n, len(content))
	}
	if fromReader != fromBytes {
		t.Fatalf("FromReader() = %q, want %q", fromReader, fromBytes)
	}
}
