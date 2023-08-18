package sshexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"

	"golang.org/x/crypto/ssh"
)

var ErrUnknownHostKey = errors.New("unknown host key")

type Authority interface {
	GetAccessDetails(ctx context.Context, target string) (*AccessDetails, error)
}

type AccessDetails struct {
	User          string
	Address       string
	KnownHostKeys []ssh.PublicKey
	Cert          *ssh.Certificate
	Signer        ssh.Signer
}

func (d *AccessDetails) NewClientConfig(opts ...func(*ssh.ClientConfig)) (*ssh.ClientConfig, error) {
	if d.Signer == nil {
		return nil, errors.New("nil Signer")
	}

	signer := d.Signer

	if d.Cert != nil {
		certSigner, err := ssh.NewCertSigner(d.Cert, d.Signer)
		if err != nil {
			return nil, err
		}
		signer = certSigner
	}

	config := &ssh.ClientConfig{
		// The secure default: we'll check against the specified known host
		// keys, if any. If none specified, SSH handshake will fail with
		// ErrUnknownHostKey. If necessary, the caller may specify their own
		// ssh.HostKeyCallback.
		HostKeyCallback: d.HostKeyCallback,
	}

	for _, o := range opts {
		o(config)
	}

	config.User = d.User
	// This is by design: we only use public key authentication method.
	config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}

	return config, nil
}

func (d *AccessDetails) HostKeyCallback(_ string, _ net.Addr, hostKey ssh.PublicKey) error {
	if hostKey == nil {
		return fmt.Errorf("got a nil host key")
	}

	got := hostKey.Marshal()

	for _, known := range d.KnownHostKeys {
		if want := known.Marshal(); bytes.Equal(got, want) {
			return nil // We've got a matching host key!
		}
	}

	return fmt.Errorf("%w: %s", ErrUnknownHostKey,
		bytes.TrimSpace(ssh.MarshalAuthorizedKey(hostKey)))
}

func New(
	ctx context.Context, a Authority, target string, opts ...func(*ssh.ClientConfig),
) (*ssh.Client, error) {
	ad, err := a.GetAccessDetails(ctx, target)
	if err != nil {
		return nil, err
	}

	config, err := ad.NewClientConfig(opts...)
	if err != nil {
		return nil, err
	}

	d := net.Dialer{Timeout: config.Timeout}
	conn, err := d.DialContext(ctx, "tcp", ad.Address)
	if err != nil {
		return nil, err
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, ad.Address, config)
	if err != nil {
		return nil, errors.Join(err, conn.Close())
	}

	return ssh.NewClient(c, chans, reqs), nil
}
