package main

import (
	"embed"
	"log"
	"net/http"

	"github.com/suyash01/splitease/internal/config"
	"github.com/suyash01/splitease/internal/database"
	"github.com/suyash01/splitease/internal/handlers"
	"github.com/suyash01/splitease/internal/web"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

//go:embed web/templates/*
var templates embed.FS

func main() {
	cfg := config.Load()

	database.Connect(cfg)
	defer database.Close()

	database.RunMigrations(embedMigrations)

	web.InitTemplates(templates)

	http.HandleFunc("/", handlers.HandleHome)
	http.HandleFunc("/expenses", handlers.HandleExpenses)
	http.HandleFunc("/expenses/add", handlers.HandleAddExpense)
	http.HandleFunc("/expenses/delete/", handlers.HandleDeleteExpense)
	http.HandleFunc("/balances", handlers.HandleBalances)
	http.HandleFunc("/users", handlers.HandleUsers)
	http.HandleFunc("/users/add", handlers.HandleAddUser)
	http.HandleFunc("/users/delete/", handlers.HandleDeleteUser)

	log.Printf("Server starting on port %s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}
