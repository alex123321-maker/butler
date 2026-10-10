// Package egress is the shared network boundary for model-selected resources.
package egress

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

const MaxRequestBytes int64 = 4 * 1024 * 1024

var ErrDenied = errors.New("outbound destination denied by policy")
var ErrLimit = errors.New("outbound request byte allowance exceeded")

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}
type DialFunc func(context.Context, string, string) (net.Conn, error)

// Resolver and Dial are trusted construction seams, never model arguments.
// Empty domains permit public internet destinations, never private networks.
type Policy struct {
	AllowedDomains []string
	AllowedPorts   []string
	Resolver       Resolver
	Dial           DialFunc
}

func (p Policy) ValidateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Opaque != "" || u.Hostname() == "" || strings.ContainsAny(u.Hostname(), "%\\") || u.Fragment != "" {
		return nil, ErrDenied
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	allowedPort := port == "80" || port == "443"
	for _, v := range p.AllowedPorts {
		if v == port {
			allowedPort = true
		}
	}
	if !allowedPort || !domainAllowed(u.Hostname(), p.AllowedDomains) {
		return nil, ErrDenied
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !PublicAddress(ip) {
		return nil, ErrDenied
	}
	return u, nil
}

// Resolve checks every answer. Dial uses this same checked answer, so a second
// DNS lookup cannot replace a public address with an internal recipient.
func (p Policy) Resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if !domainAllowed(host, p.AllowedDomains) {
		return nil, ErrDenied
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !PublicAddress(ip) {
			return nil, ErrDenied
		}
		return []netip.Addr{ip}, nil
	}
	resolver := p.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	answers, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(answers) == 0 {
		return nil, ErrDenied
	}
	addresses := make([]netip.Addr, 0, len(answers))
	for _, answer := range answers {
		ip, ok := netip.AddrFromSlice(answer.IP)
		if !ok || !PublicAddress(ip) || answer.Zone != "" {
			return nil, ErrDenied
		}
		addresses = append(addresses, ip.Unmap())
	}
	return addresses, nil
}

func domainAllowed(host string, domains []string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if len(domains) == 0 {
		return true
	}
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if host == domain {
			return true
		}
		if strings.HasPrefix(domain, "*.") && strings.HasSuffix(host, "."+strings.TrimPrefix(domain, "*.")) {
			return true
		}
	}
	return false
}

var deniedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"), netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("2001::/32"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
}

func PublicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, prefix := range deniedPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}
