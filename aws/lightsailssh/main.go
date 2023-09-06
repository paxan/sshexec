package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
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

func regionFlag(fs *flag.FlagSet, ptr *string) {
	fs.StringVar(ptr, "region", "", "AWS region to use")
}

func profileFlag(fs *flag.FlagSet, ptr *string) {
	fs.StringVar(ptr, "profile", "", "AWS CLI profile")
}

func mfaCodeFlag(fs *flag.FlagSet, ptr *string) {
	fs.StringVar(ptr, "mfa", "", "valid MFA `code` to refresh AWS credentials")
}

func apiEndpointFlag(fs *flag.FlagSet, ptr *string) {
	fs.StringVar(ptr, "endpoint-url", "", "override the default API `URL` with the given URL")
}

func awsConfig(ctx context.Context, profile, region, mfaCode string) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
		config.WithRegion(region),
		config.WithAssumeRoleCredentialOptions(func(o *stscreds.AssumeRoleOptions) {
			if mfaCode != "" {
				o.TokenProvider = func() (string, error) { return mfaCode, nil }
			}
		}),
	)
}

func main() {
	log.SetFlags(0)

	if inRsyncMode() {
		if err := rsyncMain("rsync("+os.Args[0]+")", os.Args[1:]); err != nil {
			log.Fatal(err)
		}
		return
	}

	var (
		profile     string
		region      string
		mfaCode     string
		apiEndpoint string
		instance    string
		commands    stringsFlag
	)

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	profileFlag(fs, &profile)
	regionFlag(fs, &region)
	mfaCodeFlag(fs, &mfaCode)
	apiEndpointFlag(fs, &apiEndpoint)
	fs.StringVar(&instance, "i", "", "the `name` of a Lightsail instance")
	fs.Var(&commands, "c", "`command` to execute, can be specified more than once")

	fs.Parse(os.Args[1:])

	if instance == "" {
		f := fs.Lookup("i")
		_, usage := flag.UnquoteUsage(f)
		log.Printf("%q is not valid as %s", f.Value, usage)
		fs.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	cfg, err := awsConfig(ctx, profile, region, mfaCode)
	if err != nil {
		log.Fatal(err)
	}

	a := lightsail.NewAuthority(cfg, lightsail.WithBaseEndpoint(apiEndpoint))

	creds, err := a.IssueCredentials(ctx, instance)
	if err != nil {
		log.Fatal(err)
	}

	if len(commands) != 0 {
		if err := runCommands(creds, commands); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := runShell(creds); err != nil {
		log.Fatal(err)
	}
}
