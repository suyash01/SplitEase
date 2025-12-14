package database

import (
	"embed"
	"log"

	"github.com/pressly/goose/v3"
)

func RunMigrations(embedMigrations embed.FS) {
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}
	if err := goose.Up(DB, "migrations"); err != nil {
		log.Fatal(err)
	}
}
