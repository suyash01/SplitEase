package handlers

import (
	"net/http"

	"github.com/suyash01/splitease/internal/database"
	"github.com/suyash01/splitease/internal/models"
	"github.com/suyash01/splitease/internal/web"
)

func HandleBalances(w http.ResponseWriter, r *http.Request) {
	rows, err := database.DB.Query(`
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

	var balances []models.Balance
	for rows.Next() {
		var b models.Balance
		if err := rows.Scan(&b.User, &b.Amount); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		balances = append(balances, b)
	}

	web.Tmpl.ExecuteTemplate(w, "balances.html", balances)
}
