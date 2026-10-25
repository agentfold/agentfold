package crypto

import (
	"bytes"
	"errors"
	"testing"
	"testing/iotest"
)

func TestSealAndOpen(t *testing.T) {
	box, err := NewBox(NewKey())
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal([]byte("token"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("token")) {
		t.Fatal("plaintext leaked")
	}
	plain, err := box.Open(sealed)
	if err != nil || string(plain) != "token" {
		t.Fatalf("Open = %q, %v", plain, err)
	}

	other, _ := NewBox(NewKey())
	if _, err := other.Open(sealed); !errors.Is(err, ErrOpen) {
		t.Fatalf("other key: %v", err)
	}
	if _, err := box.Open([]byte("short")); !errors.Is(err, ErrOpen) {
		t.Fatalf("short input: %v", err)
	}
}

func TestBadKeys(t *testing.T) {
	for _, key := range []string{"", "not base64!", "c2hvcnQ="} {
		if _, err := NewBox(key); err == nil {
			t.Errorf("NewBox(%q) accepted", key)
		}
	}
}

func TestSealFailsWithoutRandomness(t *testing.T) {
	box, _ := NewBox(NewKey())
	box.rand = iotest.ErrReader(errors.New("no entropy"))
	if _, err := box.Seal([]byte("x")); err == nil {
		t.Fatal("expected an error")
	}
}
