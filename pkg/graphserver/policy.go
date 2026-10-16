package graphserver

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/emersion/go-smtp"
)

// Relay denials are permanent (550 5.7.1, delivery not authorised), so
// clients do not retry them.
var (
	errSourceDenied = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 7, 1},
		Message:      "relay from this address is not permitted",
	}
	errSenderDenied = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 7, 1},
		Message:      "sender is not permitted",
	}
)

// relayPolicy decides which sources may relay and which senders may send.
// An empty list allows everything.
type relayPolicy struct {
	senders map[string]bool
	sources []netip.Prefix
}

// newRelayPolicy builds a relayPolicy. Each source is an IP address or a
// CIDR range; an invalid source is an error.
func newRelayPolicy(senders, sources []string) (*relayPolicy, error) {
	p := &relayPolicy{
		senders: make(map[string]bool, len(senders)),
		sources: make([]netip.Prefix, 0, len(sources)),
	}
	for _, s := range senders {
		p.senders[strings.ToLower(s)] = true
	}
	for _, s := range sources {
		prefix, err := parseSource(s)
		if err != nil {
			return nil, fmt.Errorf("invalid allowed source %q: %w", s, err)
		}
		p.sources = append(p.sources, prefix)
	}

	return p, nil
}

// parseSource parses an IP address or CIDR range as a prefix, so a single
// address is a prefix of one address.
func parseSource(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, err
		}

		return prefix.Masked(), nil
	}

	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	addr = addr.Unmap()

	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// hasSources reports whether a list of allowed sources is set.
func (p *relayPolicy) hasSources() bool {
	return len(p.sources) > 0
}

// allowSource reports whether a client connecting from addr may relay.
func (p *relayPolicy) allowSource(addr net.Addr) bool {
	if len(p.sources) == 0 {
		return true
	}

	// with a list set, an address that cannot be checked is denied
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap().WithZone("")

	for _, prefix := range p.sources {
		if prefix.Contains(ip) {
			return true
		}
	}

	return false
}

// allowSender reports whether from may send through the relay. Addresses
// are compared case-insensitively.
func (p *relayPolicy) allowSender(from string) bool {
	if len(p.senders) == 0 {
		return true
	}

	return p.senders[strings.ToLower(from)]
}
