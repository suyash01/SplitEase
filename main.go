package main

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

//go:embed templates/*
var templates embed.FS

var db *sql.DB
var tmpl *template.Template

type Expense struct {
	ID          int
	Description string
	Amount      float64
	PaidBy      string
	CreatedAt   time.Time
}

type Split struct {
	ID        int
	ExpenseID int
	UserName  string
	Amount    float64
}

type Balance struct {
	User   string
	Amount float64
}

type User struct {
	ID        int
	Name      string
	CreatedAt time.Time
}

func main() {
	var err error

	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPass := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "splitwise")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPass, dbName)

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	// Run migrations
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		log.Fatal(err)
	}

	// Create template with custom functions
	funcMap := template.FuncMap{
		"abs": func(n float64) float64 {
			if n < 0 {
				return -n
			}
			return n
		},
	}
	tmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(templates, "templates/*.html"))

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/expenses", handleExpenses)
	http.HandleFunc("/expenses/add", handleAddExpense)
	http.HandleFunc("/expenses/delete/", handleDeleteExpense)
	http.HandleFunc("/balances", handleBalances)
	http.HandleFunc("/users", handleUsers)
	http.HandleFunc("/users/add", handleAddUser)
	http.HandleFunc("/users/delete/", handleDeleteUser)

	port := getEnv("PORT", "8080")
	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	tmpl.ExecuteTemplate(w, "index.html", nil)
}

func handleExpenses(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, description, amount, paid_by, created_at 
		FROM expenses 
		ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var expenses []Expense
	for rows.Next() {
		var e Expense
		if err := rows.Scan(&e.ID, &e.Description, &e.Amount, &e.PaidBy, &e.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		expenses = append(expenses, e)
	}

	tmpl.ExecuteTemplate(w, "expenses.html", expenses)
}

func handleAddExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	description := r.FormValue("description")
	amountStr := r.FormValue("amount")
	paidBy := r.FormValue("paid_by")
	splitWith := r.Form["split_with"]

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		http.Error(w, "Invalid amount", http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var expenseID int
	err = tx.QueryRow(`
		INSERT INTO expenses (description, amount, paid_by) 
		VALUES ($1, $2, $3) 
		RETURNING id
	`, description, amount, paidBy).Scan(&expenseID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add payer to split list if not already there
	hasPayer := false
	for _, user := range splitWith {
		if user == paidBy {
			hasPayer = true
			break
		}
	}
	if !hasPayer {
		splitWith = append(splitWith, paidBy)
	}

	splitAmount := amount / float64(len(splitWith))

	for _, user := range splitWith {
		_, err = tx.Exec(`
			INSERT INTO splits (expense_id, user_name, amount) 
			VALUES ($1, $2, $3)
		`, expenseID, user, splitAmount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "expenseAdded")
	handleExpenses(w, r)
}

func handleDeleteExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/expenses/delete/"):]

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM splits WHERE expense_id = $1", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec("DELETE FROM expenses WHERE id = $1", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "expenseDeleted")
	w.WriteHeader(http.StatusOK)
}

func handleBalances(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT 
			user_name,
			SUM(CASE WHEN user_name = e.paid_by THEN e.amount - s.amount ELSE -s.amount END) as balance
		FROM splits s
		JOIN expenses e ON s.expense_id = e.id
		GROUP BY user_name
		HAVING SUM(CASE WHEN user_name = e.paid_by THEN e.amount - s.amount ELSE -s.amount END) != 0
		ORDER BY balance DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var balances []Balance
	for rows.Next() {
		var b Balance
		if err := rows.Scan(&b.User, &b.Amount); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		balances = append(balances, b)
	}

	tmpl.ExecuteTemplate(w, "balances.html", balances)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, name, created_at FROM users ORDER BY name")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	// Check if request wants checkboxes or full list
	if r.Header.Get("HX-Target") == "split-checkboxes" {
		tmpl.ExecuteTemplate(w, "user-checkboxes.html", users)
	} else {
		tmpl.ExecuteTemplate(w, "user-list.html", users)
	}
}

func handleAddUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	_, err := db.Exec("INSERT INTO users (name) VALUES ($1)", name)
	if err != nil {
		// Check if it's a duplicate
		if err.Error() == "pq: duplicate key value violates unique constraint \"users_name_key\"" {
			http.Error(w, "User already exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "userAdded")
	handleUsers(w, r)
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/users/delete/"):]

	// Check if user has any expenses or splits
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT 1 FROM expenses WHERE paid_by = (SELECT name FROM users WHERE id = $1)
			UNION
			SELECT 1 FROM splits WHERE user_name = (SELECT name FROM users WHERE id = $1)
		) AS combined
	`, id).Scan(&count)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if count > 0 {
		http.Error(w, "Cannot delete user with existing expenses", http.StatusConflict)
		return
	}

	_, err = db.Exec("DELETE FROM users WHERE id = $1", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "userDeleted")
	w.WriteHeader(http.StatusOK)
}
