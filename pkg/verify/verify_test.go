package verify

import (
	"context"
	"strings"
	"testing"
)

// Known SHA-256 / SHA-512 vectors.
const (
	sha256Empty = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	sha512Empty = "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"
	sha256Hello = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
)

func TestCompute(t *testing.T) {
	ctx := context.Background()
	v := NewVerifier()

	t.Run("sha-256 of empty string", func(t *testing.T) {
		got, err := v.Compute(ctx, []byte{}, []string{"sha-256"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["sha-256"] != sha256Empty {
			t.Errorf("sha-256('') = %q, want %q", got["sha-256"], sha256Empty)
		}
	})

	t.Run("sha-512 of empty string", func(t *testing.T) {
		got, err := v.Compute(ctx, []byte{}, []string{"sha-512"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["sha-512"] != sha512Empty {
			t.Errorf("sha-512('') = %q, want %q", got["sha-512"], sha512Empty)
		}
	})

	t.Run("sha-256 of hello", func(t *testing.T) {
		got, err := v.Compute(ctx, []byte("hello"), []string{"sha-256"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["sha-256"] != sha256Hello {
			t.Errorf("sha-256('hello') = %q, want %q", got["sha-256"], sha256Hello)
		}
	})

	t.Run("multiple algorithms at once", func(t *testing.T) {
		got, err := v.Compute(ctx, []byte{}, []string{"sha-256", "sha-512"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["sha-256"] != sha256Empty {
			t.Errorf("sha-256 mismatch: got %q", got["sha-256"])
		}
		if got["sha-512"] != sha512Empty {
			t.Errorf("sha-512 mismatch: got %q", got["sha-512"])
		}
	})

	t.Run("unknown algorithm returns error", func(t *testing.T) {
		_, err := v.Compute(ctx, []byte("hello"), []string{"md5"})
		if err == nil {
			t.Fatal("expected error for unknown algorithm, got nil")
		}
		if !strings.Contains(err.Error(), "md5") {
			t.Errorf("error should mention the unknown algorithm, got: %v", err)
		}
	})

	t.Run("unknown algorithm mixed with valid returns error", func(t *testing.T) {
		_, err := v.Compute(ctx, []byte("hello"), []string{"sha-256", "sha-1"})
		if err == nil {
			t.Fatal("expected error for unknown algorithm, got nil")
		}
	})
}

func TestVerify(t *testing.T) {
	ctx := context.Background()
	v := NewVerifier()

	t.Run("correct sha-256 returns nil", func(t *testing.T) {
		err := v.Verify(ctx, []byte{}, map[string]string{"sha-256": sha256Empty})
		if err != nil {
			t.Errorf("expected nil error for correct checksum, got: %v", err)
		}
	})

	t.Run("correct sha-512 returns nil", func(t *testing.T) {
		err := v.Verify(ctx, []byte{}, map[string]string{"sha-512": sha512Empty})
		if err != nil {
			t.Errorf("expected nil error for correct checksum, got: %v", err)
		}
	})

	t.Run("correct both algorithms returns nil", func(t *testing.T) {
		err := v.Verify(ctx, []byte{}, map[string]string{
			"sha-256": sha256Empty,
			"sha-512": sha512Empty,
		})
		if err != nil {
			t.Errorf("expected nil error for correct checksums, got: %v", err)
		}
	})

	t.Run("wrong sha-256 returns error naming algorithm", func(t *testing.T) {
		err := v.Verify(ctx, []byte("hello"), map[string]string{"sha-256": sha256Empty})
		if err == nil {
			t.Fatal("expected error for wrong checksum, got nil")
		}
		if !strings.Contains(err.Error(), "sha-256") {
			t.Errorf("error should name the failing algorithm, got: %v", err)
		}
	})

	t.Run("wrong sha-512 returns error naming algorithm", func(t *testing.T) {
		err := v.Verify(ctx, []byte("hello"), map[string]string{"sha-512": sha512Empty})
		if err == nil {
			t.Fatal("expected error for wrong checksum, got nil")
		}
		if !strings.Contains(err.Error(), "sha-512") {
			t.Errorf("error should name the failing algorithm, got: %v", err)
		}
	})

	t.Run("multiple wrong algorithms error mentions all", func(t *testing.T) {
		err := v.Verify(ctx, []byte("hello"), map[string]string{
			"sha-256": sha256Empty, // wrong for "hello"
			"sha-512": sha512Empty, // wrong for "hello"
		})
		if err == nil {
			t.Fatal("expected error for wrong checksums, got nil")
		}
		if !strings.Contains(err.Error(), "sha-256") {
			t.Errorf("error should mention sha-256, got: %v", err)
		}
		if !strings.Contains(err.Error(), "sha-512") {
			t.Errorf("error should mention sha-512, got: %v", err)
		}
	})

	t.Run("empty expected map returns nil", func(t *testing.T) {
		err := v.Verify(ctx, []byte("hello"), map[string]string{})
		if err != nil {
			t.Errorf("expected nil for empty expected map, got: %v", err)
		}
	})

	t.Run("nil expected map returns nil", func(t *testing.T) {
		err := v.Verify(ctx, []byte("hello"), nil)
		if err != nil {
			t.Errorf("expected nil for nil expected map, got: %v", err)
		}
	})

	t.Run("one correct one wrong reports only the failure", func(t *testing.T) {
		err := v.Verify(ctx, []byte{}, map[string]string{
			"sha-256": sha256Empty, // correct for ""
			"sha-512": sha256Empty, // wrong: using sha-256 digest for sha-512 slot
		})
		if err == nil {
			t.Fatal("expected error for one wrong checksum, got nil")
		}
		if !strings.Contains(err.Error(), "sha-512") {
			t.Errorf("error should mention sha-512, got: %v", err)
		}
	})

	t.Run("unknown algorithm is silently skipped when a known algorithm passes", func(t *testing.T) {
		err := v.Verify(ctx, []byte{}, map[string]string{
			"sha-256":  sha256Empty, // correct — supported
			"sha3-512": "somehash",  // unknown — should be skipped
		})
		if err != nil {
			t.Errorf("expected nil when known algorithm passes and unknown is skipped, got: %v", err)
		}
	})

	t.Run("all-unknown algorithms returns error", func(t *testing.T) {
		err := v.Verify(ctx, []byte("hello"), map[string]string{
			"sha3-256": "abc",
			"btih":     "def",
		})
		if err == nil {
			t.Fatal("expected error when no supported algorithm is found, got nil")
		}
	})
}
