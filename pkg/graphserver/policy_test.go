package graphserver

import (
	"net"
	"testing"
)

func mustRelayPolicy(t *testing.T, senders, sources []string) *relayPolicy {
	t.Helper()

	p, err := newRelayPolicy(senders, sources)
	if err != nil {
		t.Fatalf("newRelayPolicy() error = %v", err)
	}

	return p
}

func tcpAddr(ip string) net.Addr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: 25}
}

// badAddr is a net.Addr whose String has no parseable host.
type badAddr struct{}

func (badAddr) Network() string { return "unknown" }
func (badAddr) String() string  { return "not-an-address" }

func TestRelayPolicyAllowSource(t *testing.T) {
	tests := []struct {
		name    string
		sources []string
		addr    net.Addr
		want    bool
	}{
		{"empty list allows all", nil, tcpAddr("192.0.2.1"), true},
		{"listed address", []string{"192.0.2.1", "192.0.2.2"}, tcpAddr("192.0.2.2"), true},
		{"unlisted address", []string{"192.0.2.1"}, tcpAddr("192.0.2.9"), false},
		{"listed ipv6 address", []string{"2001:db8::1"}, tcpAddr("2001:db8::1"), true},
		{"unparseable address with list", []string{"192.0.2.1"}, badAddr{}, false},
		{"address in cidr range", []string{"192.0.2.0/24"}, tcpAddr("192.0.2.77"), true},
		{"address outside cidr range", []string{"192.0.2.0/24"}, tcpAddr("198.51.100.1"), false},
		{"ipv6 address in cidr range", []string{"2001:db8::/32"}, tcpAddr("2001:db8:1::5"), true},
		{"ipv4-mapped ipv6 address", []string{"192.0.2.1"}, tcpAddr("::ffff:192.0.2.1"), true},
		{"unparseable address without list", nil, badAddr{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustRelayPolicy(t, nil, tt.sources)
			if got := p.allowSource(tt.addr); got != tt.want {
				t.Errorf("allowSource(%v) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestRelayPolicyAllowSender(t *testing.T) {
	tests := []struct {
		name    string
		senders []string
		from    string
		want    bool
	}{
		{"empty list allows all", nil, "alice@example.com", true},
		{"listed sender", []string{"alice@example.com", "bob@example.com"}, "bob@example.com", true},
		{"unlisted sender", []string{"alice@example.com"}, "mallory@example.com", false},
		{"sender differs in case", []string{"Alice@Example.com"}, "alice@EXAMPLE.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustRelayPolicy(t, tt.senders, nil)
			if got := p.allowSender(tt.from); got != tt.want {
				t.Errorf("allowSender(%q) = %v, want %v", tt.from, got, tt.want)
			}
		})
	}
}

func TestNewRelayPolicyInvalidSource(t *testing.T) {
	for _, source := range []string{"not-an-ip", "192.0.2.0/33", "192.0.2.300"} {
		if _, err := newRelayPolicy(nil, []string{source}); err == nil {
			t.Errorf("newRelayPolicy(%q) error = nil, want error", source)
		}
	}
}
