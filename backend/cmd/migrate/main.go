// Command migrate manages the embedded PostgreSQL schema.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "migration command failed")
		os.Exit(1)
	}
}

func run(args []string) (err error) {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.Usage = func() {
		flags.SetOutput(os.Stdout)
		fmt.Print("Usage: migrate <up|down|version|force VERSION>\nDatabase connection: SCYG_DATABASE_DSN environment variable\n")
	}
	if parseErr := flags.Parse(args); parseErr != nil {
		return fmt.Errorf("parse flags: %w", parseErr)
	}
	rest := flags.Args()
	dsn := os.Getenv("SCYG_DATABASE_DSN")
	if dsn == "" {
		return fmt.Errorf("SCYG_DATABASE_DSN is required")
	}
	if len(rest) == 0 {
		return fmt.Errorf("command is required: up, down, version, force")
	}
	if rest[0] == "force" && len(rest) != 2 {
		return fmt.Errorf("force requires version")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	if pingErr := db.PingContext(context.Background()); pingErr != nil {
		return fmt.Errorf("ping database")
	}
	runner, err := migrations.New(db, "")
	if err != nil {
		return fmt.Errorf("create migration runner: %w", err)
	}
	defer func() { err = errors.Join(err, runner.Close()) }()
	switch rest[0] {
	case "up":
		return runner.Up()
	case "down":
		return runner.Down()
	case "version":
		version, dirty, versionErr := runner.Version()
		if versionErr == nil {
			fmt.Printf("%d dirty=%t\n", version, dirty)
		}
		return versionErr
	case "force":
		version, parseErr := strconv.Atoi(rest[1])
		if parseErr != nil {
			return fmt.Errorf("invalid version")
		}
		return runner.Force(version)
	default:
		return fmt.Errorf("unknown command")
	}
}
