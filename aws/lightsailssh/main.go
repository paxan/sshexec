package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
)

func main() {
	logger := log.New(os.Stderr, "", 0)

	app := App{
		Args: os.Args,
		Log:  logger,
	}

	err := app.Run(context.Background())
	if errors.Is(err, flag.ErrHelp) {
		// Nothing to do.
	} else if err != nil {
		logger.Fatal(err)
	}
}
