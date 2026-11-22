// Command admin creates one staff account per invocation.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Planck-code/SupportFlow-AI/internal/config"
	"github.com/Planck-code/SupportFlow-AI/internal/database"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("staff account created")
}

func run(args []string) error {
	flags := flag.NewFlagSet("admin", flag.ContinueOnError)
	// Flag parser errors can echo the offending argument.
	flags.SetOutput(io.Discard)
	role := flags.String("role", "", "admin or agent")
	email := flags.String("email", "", "staff email")
	name := flags.String("display-name", "", "display name")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: admin -role admin|agent -email ADDRESS -display-name NAME (set STAFF_PASSWORD in environment)")
	}
	password := os.Getenv("STAFF_PASSWORD")
	if password == "" {
		return errors.New("STAFF_PASSWORD is required")
	}
	cfg, err := config.LoadDatabase()
	if err != nil {
		return err
	}
	db, err := database.Open(cfg)
	if err != nil {
		return errors.New("cannot initialize database")
	}
	pool, err := db.DB()
	if err != nil {
		return errors.New("cannot access database")
	}
	defer pool.Close()
	reg := service.NewRegistration(repository.NewUsers(db))
	_, err = reg.CreateStaff(context.Background(), *email, password, *name, *role)
	switch {
	case err == nil, errors.Is(err, service.ErrEmailExists), errors.Is(err, service.ErrInvalidInput):
		return err
	default:
		return errors.New("cannot create staff account in database")
	}
}
