package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

const (
	AlgorithmSHA256 = "sha256"
	HexLength       = 64
)

// Digest is a content address produced by a cryptographic hash.
type Digest struct {
	Algorithm string
	Hex       string
}

// String returns the canonical digest form: algorithm:hex.
func (d Digest) String() string {
	return d.Algorithm + ":" + d.Hex
}

// IsZero reports whether the digest is unset.
func (d Digest) IsZero() bool {
	return d.Algorithm == "" && d.Hex == ""
}

// Validate checks that the digest uses a supported algorithm and hex length.
func (d Digest) Validate() error {
	if d.Algorithm != AlgorithmSHA256 {
		return fmt.Errorf("hash: unsupported algorithm %q", d.Algorithm)
	}
	if len(d.Hex) != HexLength {
		return fmt.Errorf("hash: invalid hex length %d, want %d", len(d.Hex), HexLength)
	}
	if _, err := hex.DecodeString(d.Hex); err != nil {
		return fmt.Errorf("hash: invalid hex encoding: %w", err)
	}
	return nil
}

// Parse decodes a digest string in "algorithm:hex" form.
func Parse(raw string) (Digest, error) {
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return Digest{}, fmt.Errorf("hash: invalid digest format %q", raw)
	}

	d := Digest{Algorithm: parts[0], Hex: parts[1]}
	if err := d.Validate(); err != nil {
		return Digest{}, err
	}
	return d, nil
}

// FromBytes computes the SHA256 digest of raw content.
func FromBytes(content []byte) Digest {
	sum := sha256.Sum256(content)
	return Digest{
		Algorithm: AlgorithmSHA256,
		Hex:       hex.EncodeToString(sum[:]),
	}
}

// FromReader streams content through SHA256 and returns the digest and byte count.
func FromReader(r io.Reader) (Digest, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return Digest{}, n, err
	}
	return Digest{
		Algorithm: AlgorithmSHA256,
		Hex:       hex.EncodeToString(h.Sum(nil)),
	}, n, nil
}
