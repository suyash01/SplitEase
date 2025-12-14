package web

import (
	"embed"
	"html/template"
)

var Tmpl *template.Template

func InitTemplates(templatesFS embed.FS) {
	funcMap := template.FuncMap{
		"abs": func(n float64) float64 {
			if n < 0 {
				return -n
			}
			return n
		},
	}
	Tmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(templatesFS, "web/templates/*.html"))
}
