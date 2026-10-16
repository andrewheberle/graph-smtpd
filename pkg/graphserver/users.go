package graphserver

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// Users holds the accounts clients may authenticate as with SMTP AUTH.
type Users struct {
	hashes map[string][]byte
}

// LoadUsers reads a users file. See ParseUsers for its format.
func LoadUsers(path string) (*Users, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open users file: %w", err)
	}
	defer func() { _ = f.Close() }()

	users, err := ParseUsers(f)
	if err != nil {
		return nil, fmt.Errorf("users file %s: %w", path, err)
	}

	return users, nil
}

// ParseUsers reads users in htpasswd format: one "username:hash" per line,
// where hash is a bcrypt hash (as made by "htpasswd -nB username"). Blank
// lines and lines starting with "#" are ignored. Any other line, including
// one with a password that is not bcrypt hashed, is an error.
func ParseUsers(r io.Reader) (*Users, error) {
	users := &Users{hashes: make(map[string][]byte)}

	scanner := bufio.NewScanner(r)
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		username, hash, ok := strings.Cut(line, ":")
		if !ok || username == "" || strings.ContainsFunc(username, unicode.IsSpace) {
			return nil, fmt.Errorf("line %d: want username:hash", n)
		}
		if _, ok := users.hashes[username]; ok {
			return nil, fmt.Errorf("line %d: user %q is listed more than once", n, username)
		}
		if !isBcrypt(hash) {
			return nil, fmt.Errorf("line %d: password for %q is not a bcrypt hash", n, username)
		}

		users.hashes[username] = []byte(hash)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

// isBcrypt reports whether hash is a bcrypt hash, in any of the "$2a$",
// "$2b$" or "$2y$" variants htpasswd and other tools produce.
func isBcrypt(hash string) bool {
	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") && !strings.HasPrefix(hash, "$2y$") {
		return false
	}
	_, err := bcrypt.Cost([]byte(hash))

	return err == nil
}

// Len returns the number of users.
func (u *Users) Len() int {
	return len(u.hashes)
}

// authenticate reports whether password is correct for username. An
// unknown username takes as long to check as a known one, so response
// times do not reveal which usernames exist.
func (u *Users) authenticate(username, password string) bool {
	hash, ok := u.hashes[username]
	if !ok {
		hash = unknownUserHash()
	}

	return bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil && ok
}

// unknownUserHash is compared against for unknown usernames.
var unknownUserHash = sync.OnceValue(func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("graph-smtpd unknown user"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}

	return hash
})
