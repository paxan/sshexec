package lightsail

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/aws/aws-sdk-go-v2/service/lightsail/types"
	"github.com/paxan/sshexec"
	"golang.org/x/crypto/ssh"
)

type Authority struct {
	Client InstanceAccessDetailsGetter
}

type InstanceAccessDetailsGetter interface {
	GetInstanceAccessDetails(
		context.Context, *lightsail.GetInstanceAccessDetailsInput, ...func(*lightsail.Options),
	) (*lightsail.GetInstanceAccessDetailsOutput, error)
}

func NewAuthority(cfg aws.Config) *Authority {
	return &Authority{Client: lightsail.NewFromConfig(cfg)}
}

func (a *Authority) IssueCredentials(
	ctx context.Context, target string,
) (*sshexec.Credentials, error) {
	iad, err := a.Client.GetInstanceAccessDetails(ctx, &lightsail.GetInstanceAccessDetailsInput{
		InstanceName: aws.String(target),
		Protocol:     types.InstanceAccessProtocolSsh,
	})
	if err != nil {
		return nil, err
	}

	known, err := parseHostKeyAttributes(iad.AccessDetails.HostKeys)
	if err != nil {
		return nil, err
	}

	cert, err := parseCertKey(aws.ToString(iad.AccessDetails.CertKey))
	if err != nil {
		return nil, err
	}

	sk, err := ssh.ParsePrivateKey([]byte(aws.ToString(iad.AccessDetails.PrivateKey)))
	if err != nil {
		return nil, err
	}

	return &sshexec.Credentials{
		User:          aws.ToString(iad.AccessDetails.Username),
		Address:       net.JoinHostPort(aws.ToString(iad.AccessDetails.IpAddress), "22"),
		KnownHostKeys: known,
		Cert:          cert,
		Signer:        sk,
	}, nil
}

func parseHostKeyAttributes(hkas []types.HostKeyAttributes) (pks []ssh.PublicKey, _ error) {
	if n := len(hkas); n != 0 {
		pks = make([]ssh.PublicKey, 0, n)
	}

	for _, hka := range hkas {
		b, err := base64.StdEncoding.DecodeString(aws.ToString(hka.PublicKey))
		if err != nil {
			return nil, err
		}

		pk, err := ssh.ParsePublicKey(b)
		if err != nil {
			return nil, err
		}

		pks = append(pks, pk)
	}

	return pks, nil
}

func parseCertKey(encodedCert string) (*ssh.Certificate, error) {
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(encodedCert))
	if err != nil {
		return nil, err
	}

	cert, ok := pk.(*ssh.Certificate)
	if !ok {
		return nil, fmt.Errorf("expected an SSH certificate but got: %T", pk)
	}

	if cert.CertType != ssh.UserCert {
		return nil, fmt.Errorf("expected an SSH user certificate (type=%v) but got: type=%v",
			ssh.UserCert, cert.CertType)
	}

	return cert, nil
}
