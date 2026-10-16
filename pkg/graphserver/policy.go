package graphserver

import "net"

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
		p.senders[s] = true
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

	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return true
	}

	return p.sources[host]
}

// allowSender reports whether from may send through the relay.
func (p *relayPolicy) allowSender(from string) bool {
	if len(p.senders) == 0 {
		return true
	}

	return p.senders[from]
}
