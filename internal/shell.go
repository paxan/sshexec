package internal

import (
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type Shell struct {
	Command string
}

func (s *Shell) HandleSession(client *ssh.Client) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// Redirects local terminal's I/O to remote interactive shell.
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	session.Stdin = os.Stdin

	inFd := int(os.Stdin.Fd())
	outFd := int(os.Stdout.Fd())

	if term.IsTerminal(outFd) {
		// If starting an interactive shell from a local terminal, first places
		// the terminal in raw mode so that characters sent to the terminal are
		// sent directly to the SSH process. This allows terminal control
		// signals like ^C and ^D, tab completion, and command navigation via
		// the arrow keys to work as expected.
		prevState, err := term.MakeRaw(inFd)
		if err != nil {
			return err
		}
		defer term.Restore(inFd, prevState)

		width, height, err := term.GetSize(outFd)
		if err != nil {
			return err
		}

		// Enables echoing (prints out characters while typing).
		modes := ssh.TerminalModes{ssh.ECHO: 1}

		// Starts an interactive terminal with ANSI coloring and the same
		// dimensions as the local terminal.
		if err := session.RequestPty("xterm-256color", height, width, modes); err != nil {
			return err
		}
	}

	if s.Command != "" {
		return session.Run(s.Command)
	}

	if err := session.Shell(); err != nil {
		return err
	}

	if err := session.Wait(); err != nil {
		return err
	}

	return nil
}
