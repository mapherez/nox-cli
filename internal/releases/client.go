// Package releases reads public NoX CLI releases without application credentials.
package releases

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const API = "https://api.github.com/repos/mapherez/nox-cli/releases/latest"
const downloads = "https://github.com/mapherez/nox-cli/releases/download/"
const maxArchive = 64 << 20
const maxBinary = 256 << 20

var ErrRelease = errors.New("Unable to verify the official CLI release. Try again later.")
var stable = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type Release struct {
	Version string `json:"version"`
}
type Client struct{ HTTP *http.Client }

func New() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{HTTP: &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !officialURL(req.URL) {
			return ErrRelease
		}
		return nil
	}}}
}
func officialURL(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch u.Hostname() {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	}
	return false
}
func Version(s string) bool { return stable.MatchString(s) }
func Newer(candidate, current string) bool {
	if !Version(candidate) || !Version(current) {
		return false
	}
	a := strings.Split(strings.TrimPrefix(candidate, "v"), ".")
	b := strings.Split(strings.TrimPrefix(current, "v"), ".")
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return len(a[i]) > len(b[i])
		}
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
func (c *Client) get(ctx context.Context, address string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, ErrRelease
	}
	request.Header.Set("User-Agent", "nox-cli-release-client")
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, ErrRelease
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrRelease
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, ErrRelease
	}
	return data, nil
}
func (c *Client) Latest(ctx context.Context) (Release, error) {
	data, err := c.get(ctx, API, 1<<20)
	if err != nil {
		return Release{}, err
	}
	var wire struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if json.Unmarshal(data, &wire) != nil || !strings.HasPrefix(wire.Tag, "v") || !Version(wire.Tag) || wire.Draft || wire.Prerelease {
		return Release{}, ErrRelease
	}
	return Release{Version: wire.Tag}, nil
}
func Asset() (string, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return "", ErrRelease
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return "", ErrRelease
	}
	suffix := ".tar.gz"
	if runtime.GOOS == "windows" {
		suffix = ".zip"
	}
	return "nox-cli_" + runtime.GOOS + "_" + runtime.GOARCH + suffix, nil
}

// Download accepts a version, never an asset URL supplied by a product or cache.
func (c *Client) Download(ctx context.Context, version, stage string) (string, error) {
	if !Version(version) || !strings.HasPrefix(version, "v") {
		return "", ErrRelease
	}
	asset, err := Asset()
	if err != nil {
		return "", err
	}
	base := downloads + version + "/"
	sums, err := c.get(ctx, base+"checksums.txt", 64<<10)
	if err != nil {
		return "", err
	}
	expected := ""
	count := 0
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			expected = fields[0]
			count++
		}
	}
	digest, err := hex.DecodeString(expected)
	if err != nil || len(digest) != 32 || count != 1 {
		return "", ErrRelease
	}
	archive, err := c.get(ctx, base+asset, maxArchive)
	if err != nil {
		return "", err
	}
	actual := sha256.Sum256(archive)
	if !strings.EqualFold(hex.EncodeToString(actual[:]), expected) {
		return "", ErrRelease
	}
	binary := "nox"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	payload, err := extract(archive, binary, strings.HasSuffix(asset, ".zip"))
	if err != nil {
		return "", err
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	target := filepath.Join(stage, binary)
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return "", ErrRelease
	}
	_, err = f.Write(payload)
	if err == nil {
		err = f.Sync()
	}
	closed := f.Close()
	if err != nil || closed != nil {
		_ = os.Remove(target)
		return "", ErrRelease
	}
	return target, nil
}
func extract(data []byte, binary string, zipped bool) ([]byte, error) {
	if zipped {
		r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil || len(r.File) != 1 {
			return nil, ErrRelease
		}
		f := r.File[0]
		if f.Name != binary || !f.Mode().IsRegular() || f.UncompressedSize64 > maxBinary {
			return nil, ErrRelease
		}
		body, err := f.Open()
		if err != nil {
			return nil, ErrRelease
		}
		defer body.Close()
		payload, err := io.ReadAll(io.LimitReader(body, maxBinary+1))
		if err != nil || len(payload) == 0 || len(payload) > maxBinary {
			return nil, ErrRelease
		}
		return payload, nil
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, ErrRelease
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	header, err := tr.Next()
	if err != nil || header.Name != binary || header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maxBinary {
		return nil, ErrRelease
	}
	payload, err := io.ReadAll(io.LimitReader(tr, maxBinary+1))
	if err != nil || len(payload) > maxBinary {
		return nil, ErrRelease
	}
	if _, err = tr.Next(); err != io.EOF {
		return nil, ErrRelease
	}
	if _, err = io.Copy(io.Discard, io.LimitReader(gz, 1024)); err != nil {
		return nil, fmt.Errorf("%w", ErrRelease)
	}
	return payload, nil
}
