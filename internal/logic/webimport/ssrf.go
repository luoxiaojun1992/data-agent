// Package webimport fetches a JS-rendered web page and extracts its readable
// text and images for KB URL import (SPEC-081). It runs entirely server-side
// (headless-chrome sidecar) so it is not subject to browser CORS restrictions.
//
// Layering: this is a logic package. SSRF validation and HTML extraction are
// pure functions (L1-testable); the importer orchestrates render + download
// behind injectable interfaces (L2-testable with mocks).
package webimport

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
)

// Sentinel errors. The KB handler maps these to HTTP status codes (SPEC-081
// §4.1): ErrInvalidURL→400, ErrSSRFBlocked→403, ErrNoContent→422,
// ErrRenderFailed→502, ErrRenderUnavailable→503.
var (
	// ErrInvalidURL marks a URL that is empty, malformed, non-http(s), or has
	// an unresolvable host.
	ErrInvalidURL = errors.New("webimport: invalid url")
	// ErrSSRFBlocked marks a URL whose host resolves to a loopback/private/
	// link-local/reserved address (SSRF protection).
	ErrSSRFBlocked = errors.New("webimport: ssrf blocked")
	// ErrNoContent marks a page whose rendered text is empty and has no images.
	ErrNoContent = errors.New("webimport: no extractable content")
	// ErrRenderFailed marks a target-site fetch failure (timeout/DNS/HTTP).
	ErrRenderFailed = errors.New("webimport: render failed")
	// ErrRenderUnavailable marks that the headless render service itself is
	// unreachable or returned a server error.
	ErrRenderUnavailable = errors.New("webimport: render service unavailable")
)

// ipResolver abstracts host→IP resolution so SSRF tests can inject a fake
// resolver (no real DNS in unit tests). *net.Resolver satisfies this interface.
type ipResolver interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

// isBlockedIP reports whether an IP is loopback, private (RFC1918/RFC4193),
// link-local (incl. cloud metadata 169.254.169.254), unspecified (0.0.0.0/::),
// or multicast/reserved (SPEC-081 §5.1).
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// Normalize IPv4-mapped IPv6 (::ffff:a.b.c.d) to plain IPv4 so the
	// private/loopback checks below actually match.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}

// ValidateURL validates scheme and resolves the host, rejecting any address
// that resolves to a blocked IP. It accepts only http/https and requires the
// host to resolve to at least one non-blocked address (SPEC-081 §5.1).
func ValidateURL(ctx context.Context, rawURL string) error {
	return validateURL(ctx, rawURL, net.DefaultResolver)
}

func validateURL(ctx context.Context, rawURL string, resolver ipResolver) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ErrInvalidURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ErrInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}
	host := u.Hostname()
	if host == "" {
		return ErrInvalidURL
	}
	ips, err := resolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return ErrInvalidURL // host resolution failure → treated as invalid (400)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return ErrSSRFBlocked
		}
	}
	return nil
}
