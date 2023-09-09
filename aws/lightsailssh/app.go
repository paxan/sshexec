package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/paxan/sshexec"
	"github.com/paxan/sshexec/aws/lightsail"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type App struct {
	Args []string

	Log interface {
		Print(v ...any)
		Printf(format string, v ...any)
	}

	UsageOutput io.Writer

	config struct {
		profile     string
		region      string
		mfaCode     string
		apiEndpoint string
		loginUser   string
		port        string
		dst         destination
		commands    []string
	}
}

func (app *App) Run(ctx context.Context) error {
	if err := app.parseArgs(); err != nil {
		return err
	}

	cfg, err := awsConfig(ctx, app.config.profile, app.config.region, app.config.mfaCode)
	if err != nil {
		return err
	}

	a := lightsail.NewAuthority(cfg, lightsail.WithBaseEndpoint(app.config.apiEndpoint))

	creds, err := a.IssueCredentials(ctx, app.config.dst.instance)
	if err != nil {
		return err
	}

	if app.config.dst.user != "" && app.config.dst.user != creds.User {
		return fmt.Errorf("login user %q does not match user %q returned by GetInstanceAccessDetails",
			app.config.dst.user, creds.User)
	}

	if len(app.config.commands) != 0 {
		if err := app.runCommands(creds); err != nil {
			return err
		}
		return nil
	}

	return app.runShell(creds)
}

const doc = `OpenSSH remote login client for Amazon Lightsail instances.

This is an SSH client for logging into a Lightsail instance
and for executing commands on a Lightsail instance. It differs
from a traditional SSH client by handling SSH host authentication
and SSH user authentication automatically via Amazon Lightsail API.
It does not require local SSH key materials and uses short-lived
SSH user certificates issued by Amazon Lightsail API.

The destination Lightsail instance must be specified as either
[user@]instanceName or a URI of the form ssh://[user@]instanceName[:port].

If a command is specified, it will be executed on the instance.
A complete command line may be specified as command, or it may have
additional arguments. If supplied, the arguments will be appended to
the command, separated by spaces, before it is sent to the instance
to be executed.

Alternative way to supply multiple commands is available: by using
one or more -cmd flags. Each -cmd command will be invoked in sequence
on the same SSH connection, terminating as soon as a command fails.

`

func (app *App) parseArgs() error {
	fs := flag.NewFlagSet(app.Args[0], flag.ContinueOnError)
	fs.SetOutput(app.UsageOutput)
	fs.Usage = func() {
		fmt.Fprintf(
			fs.Output(),
			doc+
				"Usage: %s [flags] destination [command [argument ...]]\n"+
				"Flags:\n",
			fs.Name())
		fs.PrintDefaults()
	}

	// These flags have the same usage and meaning as the corresponding flags of
	// OpenSSH client.
	fs.StringVar(&app.config.loginUser, "l", "", "the `user` to log in as on the instance")
	fs.StringVar(&app.config.port, "p", "", "`port` to connect to on the instance")

	// Non-standard flags that configure AWS-related options.
	// They do not clash with the standard OpenSSH client flags.
	appendCommands := func(s string) error {
		app.config.commands = append(app.config.commands, s)
		return nil
	}
	fs.Func("cmd", "`command` to execute, can be specified more than once", appendCommands)
	fs.StringVar(&app.config.apiEndpoint, "endpoint-url", "", "override the default API `URL` with the given URL")
	fs.StringVar(&app.config.mfaCode, "mfa", "", "valid MFA `code` to refresh AWS credentials")
	fs.StringVar(&app.config.profile, "profile", "", "AWS CLI profile")
	fs.StringVar(&app.config.region, "region", "", "AWS region to use")

	if err := fs.Parse(app.Args[1:]); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return errors.New("destination is not specified")
	}

	var err error
	app.config.dst, err = parseDestination(fs.Arg(0))
	if err != nil {
		return err
	}

	switch cmdArgs := fs.Args()[1:]; {
	case len(cmdArgs) != 0 && len(app.config.commands) != 0:
		return fmt.Errorf("command %s and -cmd flags must not be used together", shQuote(cmdArgs[0]))
	case len(cmdArgs) != 0:
		app.config.commands = []string{shJoin(cmdArgs)}
	}

	// If -l user override was given, use it instead of the user parsed from
	// destination arg.
	if app.config.loginUser != "" {
		app.config.dst.user = app.config.loginUser
	}

	// If -p port override was given, use it instead of the port parsed from
	// destination arg.
	if app.config.port != "" {
		app.config.dst.port = app.config.port
	}

	// Default to 22 if port is still unspecified.
	if app.config.dst.port == "" {
		app.config.dst.port = "22"
	}

	return nil
}

func (app *App) runCommand(client *ssh.Client, cmd string) error {
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

func (app *App) runCommands(creds *sshexec.Credentials) error {
	if len(app.config.commands) == 0 {
		return nil
	}

	client, err := sshexec.NewClient(creds, app.config.dst.port)
	if err != nil {
		return err
	}
	defer client.Close()

	for _, cmd := range app.config.commands {
		err := app.runCommand(client, cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

func (app *App) runShell(creds *sshexec.Credentials) error {
	client, err := sshexec.NewClient(creds, app.config.dst.port)
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

type destination struct {
	user, instance, port string
}

// parseDestination splits s into parts. A destination may be specified as
// either [user@]instanceName or a URI of the form ssh://[user@]instanceName[:port].
func parseDestination(s string) (d destination, _ error) {
	if strings.HasPrefix(s, "ssh://") {
		u, err := url.Parse(s)
		if err != nil {
			return d, err
		}
		return destination{
			user:     u.User.Username(),
			instance: u.Hostname(),
			port:     u.Port(),
		}, nil
	}

	d.instance = s
	if at := strings.IndexRune(s, '@'); at >= 0 {
		d.user, d.instance = s[:at], s[at+1:]
	}

	return d, nil
}
