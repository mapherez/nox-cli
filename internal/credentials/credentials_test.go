package credentials

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func TestKeyValidationNeverEchoesInput(t *testing.T) {
	valid := "noxsync_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if err := ValidateKey(valid); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"private-test-secret", valid + "x", "badpref_" + valid[8:], valid[:50] + "!", valid[:50] + "B"} {
		err := ValidateKey(key)
		if err == nil || strings.Contains(err.Error(), key) {
			t.Fatal("invalid key accepted or exposed")
		}
	}
}

func TestNativeVaultReplacement(t *testing.T) {
	if os.Getenv("NOX_CLI_TEST_NATIVE_VAULT") != "1" {
		t.Skip("Native vault test is opt-in and uses a unique disposable slot.")
	}
	n := Native{service: "nox-cli-test-" + rand.Text(), account: "api-key"}
	defer func() {
		if err := n.Delete(); err != nil {
			t.Error("native test credential cleanup failed")
		}
	}()
	first := "noxsync_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	bytes := make([]byte, 32)
	bytes[0] = 1
	second := "noxsync_" + base64.RawURLEncoding.EncodeToString(bytes)
	for _, value := range []string{first, second} {
		if err := n.Set(Secret{Origin: "http://localhost", Key: value}); err != nil {
			t.Fatal("native vault save failed", err)
		}
		loaded, err := n.Get()
		if err != nil || loaded.Key != value || loaded.Origin != "http://localhost" {
			t.Fatal("native vault read/replacement failed")
		}
	}
	if err := n.Delete(); err != nil {
		t.Fatal("native vault removal failed", err)
	}
	if _, err := n.Get(); err != ErrNotFound {
		t.Fatal("removed native credential is still available")
	}
}
