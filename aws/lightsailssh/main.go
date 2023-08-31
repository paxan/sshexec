package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/paxan/sshexec"
	"github.com/paxan/sshexec/aws/lightsail"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type stringsFlag []string

func (f stringsFlag) String() string {
	buf := new(strings.Builder)

	for i, x := range f {
		if i != 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(strconv.Quote(x))
	}

	return buf.String()
}

func (f *stringsFlag) Set(s string) error {
	*f = append(*f, s)
	return nil
}

func runCommand(client *ssh.Client, cmd string) error {
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

func runCommands(creds *sshexec.Credentials, cmds []string) error {
	if len(cmds) == 0 {
		return nil
	}

	client, err := sshexec.NewClient(creds)
	if err != nil {
		return err
	}
	defer client.Close()

	for _, cmd := range cmds {
		err := runCommand(client, cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

func runShell(creds *sshexec.Credentials) error {
	client, err := sshexec.NewClient(creds)
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
		commands stringsFlag
	}

	p := params{}

	flag.StringVar(&p.instance, "i", "", "the `name` of a Lightsail instance")
	flag.Var(&p.commands, "c", "`command` to execute, can be specified more than once")
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

	a := lightsail.NewAuthority(cfg)

	creds, err := a.IssueCredentials(ctx, p.instance)
	if err != nil {
		log.Fatal(err)
	}

	if len(p.commands) != 0 {
		if err := runCommands(creds, p.commands); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := runShell(creds); err != nil {
		log.Fatal(err)
	}
}
