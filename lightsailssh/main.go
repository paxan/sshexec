package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/paxan/sshexec"
)

const prologue = `OpenSSH remote login client for Amazon Lightsail instances.

This is an SSH client for logging into a Lightsail instance and for executing
commands on a Lightsail instance. It differs from a traditional SSH client by
handling SSH host authentication and SSH user authentication automatically with
assistance of Amazon Lightsail API. Handling of local SSH key material files is
avoided in favor of short-lived SSH user certificates issued by
GetInstanceAccessDetails API.

The destination Lightsail instance must be specified as either
[user@]instanceName or a URI of the form ssh://[user@]instanceName[:port].

If a command is specified, it will be executed on the instance. A complete
command line may be specified as command, or it may have additional arguments.
If supplied, the arguments will be appended to the command, separated by spaces,
before it is sent to the instance to be executed.

Remote shell usage:
  lightsailssh [flags] destination

Remote command usage:
  lightsailssh           [flags] destination cmd [arg1 arg2 ...]
  lightsailssh -commands [flags] destination [cmd1 cmd2 ...]

Remote subsystem usage:
  lightsailssh -s [flags] destination cmd

Flags:
`

type options struct {
	sshexec.SessionOptions

	profile     string
	region      string
	mfaCode     string
	apiEndpoint string
}

// parseArgs parses command line flags into options, and a slice of the non-flag
// arguments.
func parseArgs(osArgs []string, usageWriter io.Writer) (*options, []string, error) {
	if len(osArgs) == 0 {
		return nil, nil, errors.New("no args")
	}

	args := dropUnsupportedFlags(osArgs)

	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(usageWriter)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), prologue)
		fs.PrintDefaults()
	}

	opts := options{}

	fs.BoolVar(&opts.ArgsCommands, "commands", false,
		"treat each arg after the destination as a separate command\n"+
			"to be executed in sequence on the same SSH connection,\n"+
			"terminating as soon as a command fails")

	// These flags correspond to the flags of OpenSSH client.
	fs.StringVar(&opts.LoginUser, "l", "", "the `user` to log in as on the instance")
	fs.StringVar(&opts.Port, "p", "", "`port` to connect to on the instance")
	fs.BoolVar(&opts.WithSubsystem, "s", false,
		"request invocation of a subsystem on the remote system;\n"+
			"subsystems facilitate the use of SSH as a secure transport\n"+
			"for other applications (e.g. sftp(1)); the subsystem is\n"+
			"specified as the remote command; refer to the description\n"+
			"of SessionType in ssh_config(5) for details")

	// Flags that configure AWS-related options. They are named so that they not
	// clash with the standard OpenSSH client flags.
	fs.StringVar(&opts.apiEndpoint, "endpoint-url", "", "override the default API `URL` with the given URL")
	fs.StringVar(&opts.mfaCode, "mfa", "", "fresh MFA `code` for AWS authentication")
	fs.StringVar(&opts.profile, "profile", "", "AWS CLI profile")
	fs.StringVar(&opts.region, "region", "", "AWS region to use")

	if err := fs.Parse(args[1:]); err != nil {
		return nil, nil, err
	}

	return &opts, fs.Args(), nil
}

// Matches args like "-oPermitLocalCommand no" or "-oPermitLocalCommand=no".
var oArg = regexp.MustCompile(`^-o\S`)

// dropUnsupportedFlags returns a slice of args that has values form osArgs
// excluding unsupported ssh(1) options such as -x, -o, and so on.
func dropUnsupportedFlags(osArgs []string) (args []string) {
	drop := true
	skip := false
	for i, arg := range osArgs {
		if arg == "--" {
			drop = false
		}
		if i != 0 && drop {
			if arg == "-x" || oArg.MatchString(arg) {
				continue
			}
			if arg == "-o" || skip {
				skip = !skip
				continue
			}
		}
		args = append(args, arg)
	}
	return args
}

func run(ctx context.Context, opts *options, args []string, ls instanceAccessDetailsGetter) error {
	sh, dst, err := sshexec.NewSessionHandler(opts.SessionOptions, args)
	if err != nil {
		return err
	}

	creds, err := getInstanceCredentials(ctx, ls, dst.Instance)
	if err != nil {
		return err
	}

	if dst.User != "" && dst.User != creds.User {
		return fmt.Errorf("login user %q does not match user %q returned by GetInstanceAccessDetails",
			dst.User, creds.User)
	}

	client, err := sshexec.NewClient(creds, dst.Port)
	if err != nil {
		return err
	}

	return errors.Join(sh.HandleSession(client), client.Close())
}

func main() {
	log.SetFlags(0)

	ctx := context.Background()

	opts, args, err := parseArgs(os.Args, log.Writer())
	if errors.Is(err, flag.ErrHelp) {
		// Help has been provided.
		return
	} else if err != nil {
		log.Fatal(err)
	}

	cfg, err := loadAWSConfig(ctx, opts.profile, opts.region, opts.mfaCode)
	if err != nil {
		log.Fatal(err)
	}

	ls := lightsail.NewFromConfig(cfg, withBaseEndpoint(opts.apiEndpoint))

	err = run(ctx, opts, args, ls)
	if err != nil {
		log.Fatal(err)
	}
}
