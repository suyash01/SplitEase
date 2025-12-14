package handlers

import (
	"net/http"

	"github.com/suyash01/splitease/internal/database"
	"github.com/suyash01/splitease/internal/models"
	"github.com/suyash01/splitease/internal/web"
)

func HandleUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := database.DB.Query("SELECT id, name, created_at FROM users ORDER BY name")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Name, &u.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	// Check if request wants checkboxes or full list
	if r.Header.Get("HX-Target") == "split-checkboxes" {
		web.Tmpl.ExecuteTemplate(w, "user-checkboxes.html", users)
	} else {
		web.Tmpl.ExecuteTemplate(w, "user-list.html", users)
	}
}

func HandleAddUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	_, err := database.DB.Exec("INSERT INTO users (name) VALUES ($1)", name)
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
	HandleUsers(w, r)
}

func HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/users/delete/"):]

	// Check if user has any expenses or splits
	var count int
	err := database.DB.QueryRow(`
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

	_, err = database.DB.Exec("DELETE FROM users WHERE id = $1", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "userDeleted")
	w.WriteHeader(http.StatusOK)
}
