package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsAreNonSecretAndAtomic(t *testing.T) {
	s := Store{Root: filepath.Join(t.TempDir(), "nox-cli")}
	if value, err := s.Load(); err != nil || value.GatewayOrigin != "" {
		t.Fatal(value, err)
	}
	want := Settings{Version: 1, GatewayOrigin: "http://gateway.home.arpa", CredentialRef: CredentialRef}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || got != want {
		t.Fatal(got, err)
	}
	data, err := os.ReadFile(filepath.Join(s.Root, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "noxsync_") || strings.Contains(string(data), "apiKey") {
		t.Fatal("credential found in configuration")
	}
	entries, _ := os.ReadDir(s.Root)
	if len(entries) != 1 {
		t.Fatal("unnecessary settings staging files remain")
	}
	if err := os.WriteFile(filepath.Join(s.Root, "config.json"), []byte(`{"version":1,"gatewayOrigin":"http://localhost","apiKey":"should-not-be-accepted"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Fatal("unexpected secret configuration field accepted")
	}
}

func TestSettingsRejectNonRegularTarget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	s := Store{Root: root}
	if _, err := s.Load(); err == nil {
		t.Fatal("directory accepted as settings")
	}
	if err := s.Save(Settings{Version: 1}); err == nil {
		t.Fatal("directory overwritten")
	}
}
