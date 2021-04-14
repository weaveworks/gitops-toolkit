package git

import (
	"errors"

	"github.com/fluxcd/go-git-providers/gitprovider"
	"github.com/fluxcd/pkg/ssh/knownhosts"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// AuthMethod specifies the authentication method and related credentials for connecting
// to a Git repository.
type AuthMethod interface {
	// This AuthMethod is a superset of the go-git AuthMethod
	transport.AuthMethod
	// TransportType defines what transport type should be used with this method
	TransportType() gitprovider.TransportType
}

// NewSSHAuthMethod creates a new AuthMethod for the Git SSH protocol, using a given
// identity and known_hosts file.
//
// identityFile is the bytes of e.g. ~/.ssh/id_rsa, given that ~/.ssh/id_rsa.pub is
// registered with and trusted by the Git provider.
//
// knownHostsFile should be the file content of the known_hosts file to use for remote (e.g. GitHub)
// public key verification.
// If you want to use the default git CLI behavior, populate this byte slice with contents from
// ioutil.ReadFile("~/.ssh/known_hosts").
func NewSSHAuthMethod(identityFile, knownHostsFile []byte) (AuthMethod, error) {
	if len(identityFile) == 0 || len(knownHostsFile) == 0 {
		return nil, errors.New("invalid identityFile, knownHostsFile options")
	}

	pk, err := ssh.NewPublicKeys("git", identityFile, "")
	if err != nil {
		return nil, err
	}
	callback, err := knownhosts.New(knownHostsFile)
	if err != nil {
		return nil, err
	}
	pk.HostKeyCallback = callback

	return &authMethod{
		AuthMethod: pk,
		t:          gitprovider.TransportTypeGit,
	}, nil
}

func NewHTTPSAuthMethod(username, password string) (AuthMethod, error) {
	if len(username) == 0 || len(password) == 0 {
		return nil, errors.New("invalid username, password options")
	}
	return &authMethod{
		AuthMethod: &http.BasicAuth{
			Username: username,
			Password: password,
		},
		t: gitprovider.TransportTypeHTTPS,
	}, nil
}

type authMethod struct {
	transport.AuthMethod
	t gitprovider.TransportType
}

func (a *authMethod) TransportType() gitprovider.TransportType {
	return a.t
}

func (a *authMethod) ApplyTo(target *Options) {
	target.AuthMethod = a
}
