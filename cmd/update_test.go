package cmd

import (
	"testing"
)

func TestRunUpdate_PinAndUnpinMutuallyExclusive(t *testing.T) {
	updatePin = true
	updateUnpin = true
	defer func() {
		updatePin = false
		updateUnpin = false
	}()

	err := runUpdate(nil, []string{"github.com/casey/just"})
	if err == nil {
		t.Fatal("expected error when --pin and --unpin both set")
	}
}

func TestRunUpdate_PinRequiresPackageName(t *testing.T) {
	updatePin = true
	defer func() { updatePin = false }()

	err := runUpdate(nil, []string{})
	if err == nil {
		t.Fatal("expected error when --pin set without package names")
	}
}

func TestRunUpdate_UnpinRequiresPackageName(t *testing.T) {
	updateUnpin = true
	defer func() { updateUnpin = false }()

	err := runUpdate(nil, []string{})
	if err == nil {
		t.Fatal("expected error when --unpin set without package names")
	}
}
