package gateway

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
)

type lookupFunc func(context.Context, string, string) ([]netip.Addr, error)

func (f lookupFunc) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return f(ctx, network, host)
}

func TestPrivateOriginsAndVPNAddresses(t *testing.T) {
	for _, raw := range []string{"http://10.23.45.67:4321", "http://gateway.home.arpa/", "https://100.100.1.2", "http://100.96.1.2", "http://127.0.0.1:1234", "http://[::1]", "http://[fd7a:115c:a1e0::1]", "http://[::ffff:192.168.1.2]"} {
		if _, err := Origin(raw); err != nil {
			t.Fatalf("private origin rejected: %s %v", raw, err)
		}
	}
	for _, raw := range []string{"http://8.8.8.8", "http://[2606:4700:4700::1111]", "http://[::ffff:8.8.8.8]", "http://169.254.169.254", "http://0.0.0.0", "http://user:private-secret@localhost", "http://localhost/apps/a", "http://localhost?api_key=private-secret", "http://localhost:0", "http://localhost:65536", "file:///tmp/x"} {
		if _, err := Origin(raw); err == nil || strings.Contains(err.Error(), "private-secret") {
			t.Fatal("unsafe origin accepted or exposed", raw)
		}
	}
}

func TestDNSDestinationIsPinnedAndMixedAnswersAreRejected(t *testing.T) {
	calls := 0
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		calls++
		if address != "100.96.1.2:80" {
			t.Fatal("dial resolved a different destination", address)
		}
		return nil, errors.New("test refusal")
	}
	lookup := lookupFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("100.96.1.2")}, nil
	})
	tr := newTransport("http://gateway.home.arpa", "test-key", lookup, dial)
	tr.base.DialContext(context.Background(), "tcp", "gateway.home.arpa:80")
	if calls != 1 {
		t.Fatal("private destination was not attempted")
	}
	lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("192.168.1.1"), netip.MustParseAddr("8.8.8.8")}, nil
	}
	tr = newTransport("http://gateway.home.arpa", "test-key", lookup, dial)
	if _, err := tr.base.DialContext(context.Background(), "tcp", "gateway.home.arpa:80"); !errors.Is(err, ErrPrivate) || calls != 1 {
		t.Fatal("public DNS answer reached the dialer")
	}
	request, _ := http.NewRequest("POST", "http://other.home.arpa/mcp", nil)
	if _, err := tr.RoundTrip(request); !errors.Is(err, ErrPrivate) {
		t.Fatal("credential could cross origins")
	}
}
