package verify

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"strings"
)

// Verifier computes and verifies cryptographic checksums for byte slices.
type Verifier interface {
	// Verify checks that data matches all checksums in expected.
	// Keys in expected use the hyphenated algorithm name (e.g. "sha-256").
	// All mismatches are collected before returning a single error.
	// Returns nil if expected is nil or empty.
	Verify(ctx context.Context, data []byte, expected map[string]string) error

	// Compute returns hex-encoded checksums for data using each named algorithm.
	// Returns an error for any unrecognized algorithm name.
	Compute(ctx context.Context, data []byte, algorithms []string) (map[string]string, error)
}

type verifier struct{}

// NewVerifier returns a Verifier backed by the standard library.
func NewVerifier() Verifier {
	return &verifier{}
}

func (v *verifier) Compute(_ context.Context, data []byte, algorithms []string) (map[string]string, error) {
	result := make(map[string]string, len(algorithms))
	for _, algo := range algorithms {
		digest, err := computeOne(data, algo)
		if err != nil {
			return nil, err
		}
		result[algo] = digest
	}
	return result, nil
}

func (v *verifier) Verify(_ context.Context, data []byte, expected map[string]string) error {
	if len(expected) == 0 {
		return nil
	}

	var failures []string
	checked := 0
	for algo, want := range expected {
		got, err := computeOne(data, algo)
		if err != nil {
			// Unknown algorithm — skip; only fail on algorithms we can compute.
			continue
		}
		checked++
		if got != want {
			failures = append(failures, fmt.Sprintf("%s mismatch: expected %s, got %s", algo, want, got))
		}
	}

	// If every algorithm in expected was unknown, refuse to silently pass.
	if checked == 0 && len(expected) > 0 {
		var algos []string
		for algo := range expected {
			algos = append(algos, algo)
		}
		return fmt.Errorf("no supported checksum algorithm found in expected set; got: %s", strings.Join(algos, ", "))
	}

	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

// computeOne returns the hex-encoded digest for a single algorithm.
func computeOne(data []byte, algo string) (string, error) {
	switch algo {
	case "sha-256":
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:]), nil
	case "sha-512":
		sum := sha512.Sum512(data)
		return hex.EncodeToString(sum[:]), nil
	default:
		return "", fmt.Errorf("unsupported algorithm: %q", algo)
	}
}
