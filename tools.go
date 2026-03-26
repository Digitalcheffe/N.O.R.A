//go:build tools

// Package tools pins dependencies that are not yet imported in application
// code but are required by the project (used in later tasks).
package tools

import (
	_ "github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	_ "golang.org/x/crypto/bcrypt"
)
