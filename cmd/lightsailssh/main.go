package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	lightsailclient "github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/paxan/sshexec"
	"github.com/paxan/sshexec/lightsail"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func runCommand(ctx context.Context, a sshexec.Authority, instance string, cmd string) error {
	client, err := sshexec.New(ctx, a, instance)
	if err != nil {
		return err
	}
	defer client.Close()

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

func runShell(ctx context.Context, a sshexec.Authority, instance string) error {
	client, err := sshexec.New(ctx, a, instance)
	if err != nil {
		return err
	}
	defer client.Close()

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

	if err := session.Shell(); err != nil {
		return err
	}

	if err := session.Wait(); err != nil {
		return err
	}

	return nil
}

func main() {
	log.SetFlags(0)

	type params struct {
		instance string
		command  string
	}

	p := params{}

	flag.StringVar(&p.instance, "i", "", "the `name` of a Lightsail instance")
	flag.StringVar(&p.command, "c", "", "`command` to execute")
	flag.Parse()

	if p.instance == "" {
		f := flag.Lookup("i")
		_, usage := flag.UnquoteUsage(f)
		log.Printf("%q is not valid as %s", f.Value, usage)
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}

	la := &lightsail.Authority{Client: lightsailclient.NewFromConfig(cfg)}

	if p.command != "" {
		if err := runCommand(ctx, la, p.instance, p.command); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := runShell(ctx, la, p.instance); err != nil {
		log.Fatal(err)
	}
}
