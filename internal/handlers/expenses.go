package handlers

import (
	"net/http"
	"strconv"

	"github.com/suyash01/splitease/internal/database"
	"github.com/suyash01/splitease/internal/models"
	"github.com/suyash01/splitease/internal/web"
)

func HandleExpenses(w http.ResponseWriter, r *http.Request) {
	rows, err := database.DB.Query(`
		SELECT id, description, amount, paid_by, created_at
		FROM expenses
		ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var expenses []models.Expense
	for rows.Next() {
		var e models.Expense
		if err := rows.Scan(&e.ID, &e.Description, &e.Amount, &e.PaidBy, &e.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		expenses = append(expenses, e)
	}

	web.Tmpl.ExecuteTemplate(w, "expenses.html", expenses)
}

func HandleAddExpense(w http.ResponseWriter, r *http.Request) {
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

	tx, err := database.DB.Begin()
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
	HandleExpenses(w, r)
}

func HandleDeleteExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/expenses/delete/"):]

	tx, err := database.DB.Begin()
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
