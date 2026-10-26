//go:build windows

package secure

import (
	"bytes"
	"testing"
)

func TestDPAPIRoundTrip(t *testing.T) {
	plain := []byte("token cache secret")
	encrypted, err := Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("decrypted = %q, want %q", decrypted, plain)
	}
	if err := Check(); err != nil {
		t.Fatal(err)
	}
	if Backend() != "Windows DPAPI" {
		t.Fatalf("backend = %q", Backend())
	}
}
