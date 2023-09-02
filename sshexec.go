package sshexec

import (
	"bytes"
	"errors"
	"fmt"
	"net"

	"golang.org/x/crypto/ssh"
)

var ErrUnknownHostKey = errors.New("unknown host key")

type Credentials struct {
	User          string
	Address       string
	KnownHostKeys []ssh.PublicKey
	Cert          *ssh.Certificate
	Signer        ssh.Signer
}

func NewClient(creds *Credentials, opts ...func(*ssh.ClientConfig)) (*ssh.Client, error) {
	config, err := NewClientConfig(creds, opts...)
	if err != nil {
		return nil, err
	}
	return ssh.Dial("tcp", creds.Address, config)
}

func NewClientConfig(creds *Credentials, opts ...func(*ssh.ClientConfig)) (*ssh.ClientConfig, error) {
	if creds.Signer == nil {
		return nil, errors.New("nil Signer")
	}

	signer := creds.Signer

	if creds.Cert != nil {
		if creds.Cert.CertType != ssh.UserCert {
			return nil, fmt.Errorf("expected an SSH user certificate (type=%v) but got: type=%v",
				ssh.UserCert, creds.Cert.CertType)
		}

		certSigner, err := ssh.NewCertSigner(creds.Cert, creds.Signer)
		if err != nil {
			return nil, err
		}

		signer = certSigner
	}

	config := &ssh.ClientConfig{
		// We'll check against the specified known host keys, if any. If none
		// specified, SSH handshake will fail with ErrUnknownHostKey. If
		// necessary, the caller may specify their own ssh.HostKeyCallback.
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			return validateHostKey(key, creds.KnownHostKeys)
		},
	}

	for _, o := range opts {
		o(config)
	}

	config.User = creds.User
	// This is by design: we only use public key authentication method.
	config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}

	return config, nil
}

func validateHostKey(key ssh.PublicKey, knownHostKeys []ssh.PublicKey) error {
	if key == nil {
		return fmt.Errorf("got a nil host key")
	}

	got := key.Marshal()

	for _, known := range knownHostKeys {
		if want := known.Marshal(); bytes.Equal(got, want) {
			return nil // We've got a matching host key!
		}
	}

	return fmt.Errorf("%w: %s", ErrUnknownHostKey, bytes.TrimSpace(ssh.MarshalAuthorizedKey(key)))
}
