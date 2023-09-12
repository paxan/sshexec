package internal

import (
	"errors"
	"io"
	"os"

	"golang.org/x/crypto/ssh"
)

type Subsystem struct {
	Command string
}

func (sub *Subsystem) HandleSession(client *ssh.Client) error {

	// NOTE: This is the local side of a ssh(1) subsystem session.
	//
	// I found some obscure posts in Google Groups and terse utterances in RFCs,
	// which served as the basis for this implementation.
	//
	// It seems to work with 'scp -S ...' and 'sftp -S ...'.

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	inPipe, err := session.StdinPipe()
	if err != nil {
		return err
	}
	defer inPipe.Close()

	outPipe, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	session.Stderr = os.Stderr

	if err := session.RequestSubsystem(sub.Command); err != nil {
		return err
	}

	outPipeErr := make(chan error, 1)
	go func() {
		// Forward the remote side's stdout to the local stdout.
		_, err := io.Copy(os.Stdout, outPipe)
		outPipeErr <- err
	}()

	// Forward the local stdin to the remote side's stdin.
	_, err = io.Copy(inPipe, os.Stdin)

	// Once the stdin forwarding is done, endeavor to tell the remote side to
	// stop writing to us.
	if cw, ok := outPipe.(interface{ CloseWrite() error }); ok {
		if cwerr := cw.CloseWrite(); err != nil {
			return errors.Join(err, cwerr)
		}
	}

	return errors.Join(err, <-outPipeErr)
}
