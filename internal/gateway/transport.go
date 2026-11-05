package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrOrigin      = errors.New("Enter a Gateway origin such as http://gateway.home.arpa:5711, without a path or credentials.")
	ErrRedirect    = errors.New("The Gateway redirected this request. Enter its final private base address; redirects are not followed.")
	ErrPrivate     = errors.New("The Gateway must use a private LAN or VPN address.")
	ErrRejected    = errors.New("Your API key is no longer valid. Enter a new API key to continue.")
	ErrUnavailable = errors.New("Unable to connect to the Gateway. Check its address and your private network connection.")
	ErrProtocol    = errors.New("The Gateway does not provide compatible NoX discovery.")
	ErrRequest     = errors.New("The Gateway could not complete this request. Your saved API key has not been changed.")
)

func Origin(raw string) (string, error) {
	if len(raw) > 2048 {
		return "", ErrOrigin
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || strings.HasSuffix(u.Host, ":") || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(u.Hostname(), "%") {
		return "", ErrOrigin
	}
	u.Path = ""
	u.Host = strings.ToLower(u.Host)
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !privateAddress(ip) {
		return "", ErrPrivate
	}
	// Validate ports using the standard parser, without a network connection.
	if u.Port() != "" {
		for _, c := range u.Port() {
			if c < '0' || c > '9' {
				return "", ErrOrigin
			}
		}
		port, err := net.LookupPort("tcp", u.Port())
		if err != nil || port < 1 || port > 65535 {
			return "", ErrOrigin
		}
	}
	return u.String(), nil
}

func privateAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && (ip.IsPrivate() || ip.IsLoopback() || netip.MustParsePrefix("100.64.0.0/10").Contains(ip))
}

type resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}
type dialFunc func(context.Context, string, string) (net.Conn, error)

type privateTransport struct {
	record         atomic.Bool
	recordMu       sync.Mutex
	last           *recordedBody
	origin         string
	key            string
	base           *http.Transport
	rejected       atomic.Bool
	privateFailure atomic.Bool
	redirected     atomic.Bool
	networkFailure atomic.Bool
}

func newTransport(origin, key string, lookup resolver, dial dialFunc) *privateTransport {
	t := &privateTransport{origin: origin, key: key}
	t.base = &http.Transport{
		Proxy:                 nil,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConns:          2,
		MaxIdleConnsPerHost:   2,
		ForceAttemptHTTP2:     false,
	}
	t.base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, ErrOrigin
		}
		ips, err := lookup.LookupNetIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, ErrUnavailable
		}
		// Reject a mixed private/public answer instead of allowing a bypass by retry.
		for _, ip := range ips {
			if !privateAddress(ip) {
				t.privateFailure.Store(true)
				return nil, ErrPrivate
			}
		}
		for _, ip := range ips {
			conn, err := dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, ErrUnavailable
	}
	return t
}

func (t *privateTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme+"://"+request.URL.Host != t.origin || request.URL.Path != "/mcp" || request.URL.User != nil || request.URL.RawQuery != "" {
		t.privateFailure.Store(true)
		return nil, ErrPrivate
	}
	request = request.Clone(request.Context())
	request.Header = request.Header.Clone()
	request.Header.Set("Authorization", "Bearer "+t.key)
	response, err := t.base.RoundTrip(request)
	if err != nil {
		t.networkFailure.Store(true)
		return nil, err
	}
	if response.StatusCode == http.StatusUnauthorized {
		t.rejected.Store(true)
	}
	response.Body = &boundedBody{ReadCloser: response.Body, remaining: 8 << 20}
	if t.record.Load() {
		body := &recordedBody{ReadCloser: response.Body}
		t.recordMu.Lock()
		t.last = body
		t.recordMu.Unlock()
		response.Body = body
	}
	return response, nil
}

type boundedBody struct {
	io.ReadCloser
	remaining int64
}

func (b *boundedBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, errors.New("Gateway response exceeded the size limit.")
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	return n, err
}

type recordedBody struct {
	io.ReadCloser
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *recordedBody) Read(p []byte) (int, error) {
	n, e := b.ReadCloser.Read(p)
	b.mu.Lock()
	_, _ = b.data.Write(p[:n])
	b.mu.Unlock()
	return n, e
}
func (t *privateTransport) recorded() []byte {
	t.recordMu.Lock()
	b := t.last
	t.recordMu.Unlock()
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte{}, b.data.Bytes()...)
}
