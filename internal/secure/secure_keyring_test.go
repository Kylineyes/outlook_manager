//go:build linux || darwin

package secure

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"
)

func withFakeKeyring(t *testing.T) {
	t.Helper()
	var mu sync.Mutex
	values := map[string]string{}
	oldGet, oldSet := keyringGet, keyringSet
	keyringGet = func(service, user string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		value, ok := values[service+"/"+user]
		if !ok {
			return "", keyring.ErrNotFound
		}
		return value, nil
	}
	keyringSet = func(service, user, password string) error {
		mu.Lock()
		defer mu.Unlock()
		values[service+"/"+user] = password
		return nil
	}
	t.Cleanup(func() {
		keyringGet, keyringSet = oldGet, oldSet
	})
}

func TestEncryptDecryptWithKeyring(t *testing.T) {
	withFakeKeyring(t)
	plain := []byte("token cache secret")
	encrypted, err := Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(encrypted, []byte(magic)) {
		t.Fatalf("missing magic prefix: %x", encrypted)
	}
	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("decrypted = %q, want %q", decrypted, plain)
	}
}

func TestDecryptRejectsTamperedData(t *testing.T) {
	withFakeKeyring(t)
	encrypted, err := Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	encrypted[len(encrypted)-1] ^= 0xff
	if _, err := Decrypt(encrypted); err == nil {
		t.Fatal("expected tamper error")
	}
}

func TestDecryptRejectsWrongFormat(t *testing.T) {
	withFakeKeyring(t)
	if _, err := Decrypt([]byte("not-encrypted")); err == nil {
		t.Fatal("expected format error")
	}
}

func TestKeyPersistsAcrossCalls(t *testing.T) {
	withFakeKeyring(t)
	first, err := Encrypt([]byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encrypt([]byte("two"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("nonce should make ciphertext different")
	}
	if _, err := Decrypt(first); err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(second); err != nil {
		t.Fatal(err)
	}
}

func TestKeyringErrorIsReturned(t *testing.T) {
	oldGet, oldSet := keyringGet, keyringSet
	keyringGet = func(service, user string) (string, error) { return "", errors.New("keyring unavailable") }
	keyringSet = func(service, user, password string) error { return nil }
	t.Cleanup(func() { keyringGet, keyringSet = oldGet, oldSet })
	if _, err := Encrypt([]byte("secret")); err == nil {
		t.Fatal("expected keyring error")
	}
}

func TestCheckWithKeyring(t *testing.T) {
	withFakeKeyring(t)
	if err := Check(); err != nil {
		t.Fatal(err)
	}
	if Backend() == "" {
		t.Fatal("backend is empty")
	}
}
