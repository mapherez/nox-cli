package releases

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func packaged(t *testing.T, name string, link bool) []byte {
	t.Helper()
	var b bytes.Buffer
	if strings.HasSuffix(name, ".exe") {
		w := zip.NewWriter(&b)
		f, _ := w.Create(name)
		_, _ = f.Write([]byte("payload"))
		_ = w.Close()
		return b.Bytes()
	}
	gz := gzip.NewWriter(&b)
	w := tar.NewWriter(gz)
	header := &tar.Header{Name: name, Size: 7, Mode: 0700, Typeflag: tar.TypeReg}
	if link {
		header.Typeflag = tar.TypeSymlink
		header.Linkname = "/outside"
		header.Size = 0
	}
	_ = w.WriteHeader(header)
	if !link {
		_, _ = w.Write([]byte("payload"))
	}
	_ = w.Close()
	_ = gz.Close()
	return b.Bytes()
}
func TestOfficialReleaseDownloadAndIntegrity(t *testing.T) {
	asset, err := Asset()
	if err != nil {
		t.Fatal(err)
	}
	binary := "nox"
	if strings.HasSuffix(asset, ".zip") {
		binary += ".exe"
	}
	payload := packaged(t, binary, false)
	hash := sha256.Sum256(payload)
	sums := hex.EncodeToString(hash[:]) + "  " + asset + "\n"
	c := New()
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || !officialURL(r.URL) {
			t.Fatal("Release request carried credentials or a foreign URL")
		}
		body := `{"tag_name":"v1.2.3","draft":false,"prerelease":false}`
		switch {
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			body = sums
		case strings.HasSuffix(r.URL.Path, asset):
			body = string(payload)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	release, err := c.Latest(context.Background())
	if err != nil || release.Version != "v1.2.3" {
		t.Fatal(release, err)
	}
	stage := t.TempDir()
	source, err := c.Download(context.Background(), release.Version, stage)
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(source)
	if string(actual) != "payload" {
		t.Fatal("wrong extracted bytes")
	}
	_ = os.Remove(source)
	sums = strings.Repeat("0", 64) + "  " + asset + "\n"
	if _, err := c.Download(context.Background(), release.Version, stage); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	if _, err := os.Stat(filepath.Join(stage, binary)); !os.IsNotExist(err) {
		t.Fatal("unverified executable left behind")
	}
}
func TestArchivesRejectTraversalLinksAndExtraEntries(t *testing.T) {
	for _, entry := range []struct {
		name string
		link bool
	}{{"../nox", false}, {"nox", true}, {"other", false}} {
		if _, err := extract(packaged(t, entry.name, entry.link), "nox", false); err == nil {
			t.Fatalf("unsafe archive accepted: %+v", entry)
		}
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	_, _ = w.Create("nox.exe")
	_, _ = w.Create("extra")
	_ = w.Close()
	if _, err := extract(b.Bytes(), "nox.exe", true); err == nil {
		t.Fatal("extra ZIP entry accepted")
	}
}
func TestStableVersionsAndRedirectPolicy(t *testing.T) {
	if !Newer("v1.10.0", "v1.9.9") || Newer("v1.0.0", "v1.0.0") || Newer("v1.0.0-beta", "v0.1.0") || Newer("v1.0.0", "dev") {
		t.Fatal("invalid version ordering")
	}
	for _, target := range []string{"https://example.com/payload", "http://github.com/file", "https://user@github.com/file", "https://github.com:8443/file"} {
		u, _ := url.Parse(target)
		if err := New().HTTP.CheckRedirect(&http.Request{URL: u}, nil); err == nil {
			t.Fatal("unsafe redirect accepted", target)
		}
	}
}
