// Command content-reconcile performs a bounded, read-only content consistency check.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		var exit *exitError
		if errors.As(err, &exit) {
			fmt.Fprintln(os.Stderr, exit.Error())
			os.Exit(exit.code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

type exitError struct {
	code int
	msg  string
}

func (err *exitError) Error() string { return err.msg }

type commandOptions struct {
	configPath      string
	evidencePath    string
	evidenceRoot    string
	release         string
	phase           string
	previousAttempt string
}

func parseCommandOptions(args []string) (commandOptions, error) {
	flags := flag.NewFlagSet("content-reconcile", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var options commandOptions
	flags.StringVar(&options.configPath, "config", "", "read-only reconciliation YAML")
	flags.StringVar(&options.evidencePath, "quiesce-evidence", "", "immutable quiesce evidence JSON")
	flags.StringVar(&options.evidenceRoot, "evidence-root", "", "evidence output root")
	flags.StringVar(&options.release, "release", "", "release path segment")
	flags.StringVar(&options.phase, "phase", "", "before or after")
	flags.StringVar(&options.previousAttempt, "previous-attempt", "", "previous attempt ID for a rerun")
	if err := flags.Parse(args); err != nil {
		return commandOptions{}, err
	}
	if flags.NArg() != 0 {
		return commandOptions{}, errors.New("content-reconcile does not accept positional arguments")
	}
	if options.configPath == "" || options.evidencePath == "" || options.evidenceRoot == "" || options.release == "" || options.phase == "" {
		return commandOptions{}, errors.New("-config, -quiesce-evidence, -evidence-root, -release and -phase are required")
	}
	return options, nil
}

func run(args []string) error {
	options, err := parseCommandOptions(args)
	if err != nil {
		return err
	}
	return execute(options)
}
