package sshexec

import (
	"errors"
	"net/url"
	"strings"

	"github.com/paxan/sshexec/internal"
	"golang.org/x/crypto/ssh"
)

type SessionOptions struct {
	LoginUser           string
	Port                string
	WithSubsystem       bool
	ArgsCommands        bool
	ForcePseudoTerminal bool
}

type SessionHandler interface {
	HandleSession(*ssh.Client) error
}

type Destination struct {
	User     string
	Instance string
	Port     string
}

func NewSessionHandler(opts SessionOptions, args []string) (SessionHandler, *Destination, error) {
	if len(args) == 0 {
		return nil, nil, errors.New("destination is not specified")
	}

	dst, err := parseDestination(args[0])
	if err != nil {
		return nil, nil, err
	}

	// If -l user override was given, use it instead of the user parsed from
	// destination arg.
	if opts.LoginUser != "" {
		dst.User = opts.LoginUser
	}

	// If -p port override was given, use it instead of the port parsed from
	// destination arg.
	if opts.Port != "" {
		dst.Port = opts.Port
	}

	// Default to 22 if port is still unspecified.
	if dst.Port == "" {
		dst.Port = "22"
	}

	switch cmdArgs := args[1:]; {
	case opts.WithSubsystem && opts.ArgsCommands:
		return nil, nil, errors.New("flags -s and -commands must not be used together")
	case opts.WithSubsystem && len(cmdArgs) != 1:
		return nil, nil, errors.New("subsystem command with no arguments is needed")
	case opts.WithSubsystem:
		return &internal.Subsystem{Command: cmdArgs[0]}, dst, nil
	case opts.ArgsCommands:
		return internal.Commands(cmdArgs), dst, nil
	case len(cmdArgs) == 0 || opts.ForcePseudoTerminal:
		return &internal.Shell{Command: internal.SHJoin(cmdArgs)}, dst, nil
	default:
		return internal.Commands{internal.SHJoin(cmdArgs)}, dst, nil
	}
}

// parseDestination splits s into user, instance name, and port. A destination
// may be specified as either "[user@]instanceName" or a URI of the form
// "ssh://[user@]instanceName[:port]".
func parseDestination(s string) (*Destination, error) {
	if strings.HasPrefix(s, "ssh://") {
		u, err := url.Parse(s)
		if err != nil {
			return nil, err
		}
		return &Destination{
			User:     u.User.Username(),
			Instance: u.Hostname(),
			Port:     u.Port(),
		}, nil
	}

	d := Destination{Instance: s}
	if at := strings.IndexRune(s, '@'); at >= 0 {
		d.User, d.Instance = s[:at], s[at+1:]
	}

	return &d, nil
}
