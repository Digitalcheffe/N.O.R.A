// nora-cli — administrative command-line tool for NORA.
//
// Runs standalone inside the container (no server process required).
// Connects directly to the SQLite database via NORA_DB_PATH.
//
// Usage:
//
//	nora-cli password reset -account <email> -password <newpassword>
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/migrations"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "password":
		runPasswordCmd(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "NORA CLI — administrative tools")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  nora-cli password reset -account <email> -password <newpassword>")
}

func runPasswordCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: nora-cli password reset -account <email> -password <newpassword>")
		os.Exit(1)
	}
	switch args[0] {
	case "reset":
		runPasswordReset(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown password subcommand %q\n", args[0])
		fmt.Fprintln(os.Stderr, "usage: nora-cli password reset -account <email> -password <newpassword>")
		os.Exit(1)
	}
}

func runPasswordReset(args []string) {
	fs := flag.NewFlagSet("password reset", flag.ExitOnError)
	account := fs.String("account", "", "email address of the account to update")
	password := fs.String("password", "", "new password to set")
	_ = fs.Parse(args)

	if *account == "" {
		fmt.Fprintln(os.Stderr, "error: -account is required")
		fmt.Fprintln(os.Stderr, "usage: nora-cli password reset -account <email> -password <newpassword>")
		os.Exit(1)
	}
	if *password == "" {
		fmt.Fprintln(os.Stderr, "error: -password is required")
		fmt.Fprintln(os.Stderr, "usage: nora-cli password reset -account <email> -password <newpassword>")
		os.Exit(1)
	}

	dbPath := os.Getenv("NORA_DB_PATH")
	if dbPath == "" {
		dbPath = "/data/nora.db"
	}

	db, err := repo.Open(&config.Config{DBPath: dbPath}, migrations.Files)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not open database at %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	userRepo := repo.NewUserRepo(db)
	ctx := context.Background()

	user, _, err := userRepo.GetByEmail(ctx, *account)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "error: no account found for %q\n", *account)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "error: database lookup failed: %v\n", err)
		os.Exit(1)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to hash password: %v\n", err)
		os.Exit(1)
	}

	if err := userRepo.UpdatePassword(ctx, user.ID, string(hashed)); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to update password: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Password updated successfully for %s (%s)\n", user.Email, user.Role)
}
