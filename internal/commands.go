package internal

import (
	"os"

	"golang.org/x/crypto/ssh"
)

type Commands []string

func (c Commands) HandleSession(client *ssh.Client) error {
	for _, cmd := range c {
		err := c.runCommand(client, cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c Commands) runCommand(client *ssh.Client, cmd string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	return session.Run(cmd)
}
