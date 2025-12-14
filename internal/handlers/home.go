package handlers

import (
	"net/http"

	"github.com/suyash01/splitease/internal/web"
)

func HandleHome(w http.ResponseWriter, r *http.Request) {
	web.Tmpl.ExecuteTemplate(w, "index.html", nil)
}
