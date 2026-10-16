package graphserver

import (
	"github.com/emersion/go-sasl"
)

// loginServer implements the server side of the LOGIN SASL mechanism, which
// go-sasl only provides for clients. Many devices that cannot use PLAIN
// still support LOGIN: the server prompts for the username and then the
// password, each sent as its own response.
type loginServer struct {
	authenticate func(username, password string) error
	username     string
	step         int
}

func newLoginServer(authenticate func(username, password string) error) sasl.Server {
	return &loginServer{authenticate: authenticate}
}

func (l *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	switch l.step {
	case 0:
		// a client may send the username with the AUTH command itself
		if response == nil {
			l.step = 1
			return []byte("Username:"), false, nil
		}
		l.username = string(response)
		l.step = 2
		return []byte("Password:"), false, nil
	case 1:
		l.username = string(response)
		l.step = 2
		return []byte("Password:"), false, nil
	case 2:
		l.step = 3
		return nil, true, l.authenticate(l.username, string(response))
	default:
		return nil, false, sasl.ErrUnexpectedClientResponse
	}
}
