package webimport

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		name string
		ip   net.IP
		want bool
	}{
		{"nil", nil, true},
		{"loopback v4", net.ParseIP("127.0.0.1"), true},
		{"loopback v6", net.ParseIP("::1"), true},
		{"private 10", net.ParseIP("10.0.0.1"), true},
		{"private 172.16", net.ParseIP("172.16.0.1"), true},
		{"private 192.168", net.ParseIP("192.168.1.1"), true},
		{"private v6 fc00", net.ParseIP("fc00::1"), true},
		{"link-local 169.254 (cloud metadata)", net.ParseIP("169.254.169.254"), true},
		{"link-local v6 fe80", net.ParseIP("fe80::1"), true},
		{"unspecified 0.0.0.0", net.ParseIP("0.0.0.0"), true},
		{"unspecified v6", net.ParseIP("::"), true},
		{"multicast 224", net.ParseIP("224.0.0.1"), true},
		{"public 8.8.8.8", net.ParseIP("8.8.8.8"), false},
		{"public 1.1.1.1", net.ParseIP("1.1.1.1"), false},
		{"public v6", net.ParseIP("2001:4860:4860::8888"), false},
		{"mapped v4 loopback", net.ParseIP("::ffff:127.0.0.1"), true},
		{"mapped v4 public", net.ParseIP("::ffff:8.8.8.8"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBlockedIP(tc.ip); got != tc.want {
				t.Errorf("isBlockedIP(%v) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

type fakeResolver struct {
	ips []net.IP
	err error
}

func (f fakeResolver) LookupIP(_ context.Context, _, _ string) ([]net.IP, error) {
	return f.ips, f.err
}

func TestValidateURL(t *testing.T) {
	ctx := context.Background()

	publicResolver := fakeResolver{ips: []net.IP{net.ParseIP("8.8.8.8")}}
	privateResolver := fakeResolver{ips: []net.IP{net.ParseIP("10.0.0.1")}}
	mixedResolver := fakeResolver{ips: []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("192.168.1.1")}}
	failResolver := fakeResolver{err: errors.New("no such host")}
	emptyResolver := fakeResolver{}

	cases := []struct {
		name     string
		url      string
		resolver ipResolver
		wantErr  error
	}{
		{"empty", "", publicResolver, ErrInvalidURL},
		{"whitespace only", "   ", publicResolver, ErrInvalidURL},
		{"non http scheme", "ftp://example.com", publicResolver, ErrInvalidURL},
		{"no host", "https://", publicResolver, ErrInvalidURL},
		{"malformed", "://bad", publicResolver, ErrInvalidURL},
		{"dns failure", "https://example.com", failResolver, ErrInvalidURL},
		{"no ips", "https://example.com", emptyResolver, ErrInvalidURL},
		{"private host", "http://10.0.0.1", privateResolver, ErrSSRFBlocked},
		{"loopback host", "http://127.0.0.1", fakeResolver{ips: []net.IP{net.ParseIP("127.0.0.1")}}, ErrSSRFBlocked},
		{"cloud metadata", "http://169.254.169.254/latest/meta-data", fakeResolver{ips: []net.IP{net.ParseIP("169.254.169.254")}}, ErrSSRFBlocked},
		{"mixed (one private)", "https://example.com", mixedResolver, ErrSSRFBlocked},
		{"public ok", "https://example.com", publicResolver, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateURL(ctx, tc.url, tc.resolver)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("validateURL(%q) = %v, want %v", tc.url, err, tc.wantErr)
			}
		})
	}
}
