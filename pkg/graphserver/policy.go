package graphserver

import (
	"net"
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

// relayPolicy decides which sources may connect and which senders may send.
// An empty list allows everything.
type relayPolicy struct {
	senders map[string]bool
	sources map[string]bool
}

func newRelayPolicy(senders, sources []string) (*relayPolicy, error) {
	p := &relayPolicy{
		senders: make(map[string]bool, len(senders)),
		sources: make(map[string]bool, len(sources)),
	}
	for _, s := range senders {
		p.senders[strings.ToLower(s)] = true
	}
	for _, s := range sources {
		p.sources[s] = true
	}

	return p, nil
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

	return p.sources[host]
}

// allowSender reports whether from may send through the relay. Addresses
// are compared case-insensitively.
func (p *relayPolicy) allowSender(from string) bool {
	if len(p.senders) == 0 {
		return true
	}

	return p.senders[strings.ToLower(from)]
}
