package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mitchellh/go-ps"
	"github.com/paxan/sshexec"
	"github.com/paxan/sshexec/aws/lightsail"
)

func inRsyncMode() bool {
	parent, err := ps.FindProcess(os.Getppid())
	if err != nil {
		log.Print(err)
		return false
	}
	return strings.Contains(parent.Executable(), "rsync")
}

func rsyncMain(prog string, args []string) error {
	var (
		profile     string
		region      string
		mfaCode     string
		apiEndpoint string
		user        string
	)

	fs := flag.NewFlagSet(prog, flag.ContinueOnError)

	profileFlag(fs, &profile)
	regionFlag(fs, &region)
	mfaCodeFlag(fs, &mfaCode)
	apiEndpointFlag(fs, &apiEndpoint)
	fs.StringVar(&user, "l", "", "the `user` to log in as on the instance")

	if err := fs.Parse(args); err != nil {
		return err
	}

	args = fs.Args()

	if need, got := 2, len(args); got < need {
		return fmt.Errorf("got %v arguments, need at least %v", got, need)
	}

	instance := args[0]
	cmd := commandJoin(args[1:])

	ctx := context.Background()

	cfg, err := awsConfig(ctx, profile, region, mfaCode)
	if err != nil {
		return err
	}

	creds, err := lightsail.
		NewAuthority(cfg, lightsail.WithBaseEndpoint(apiEndpoint)).
		IssueCredentials(ctx, instance)
	if err != nil {
		return err
	}

	if user != "" && user != creds.User {
		return fmt.Errorf("login user %q does not match user %q returned by GetInstanceAccessDetails",
			user, creds.User)
	}

	client, err := sshexec.NewClient(creds)
	if err != nil {
		return err
	}
	defer client.Close()

	return runCommand(client, cmd)
}
