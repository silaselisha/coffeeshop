package client

import (
	"context"
	"net/http"
	"path"
	"text/template"
)

type Querier interface {
	RenderHomePageHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	RenderAboutPageHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
}

type Templates struct {
	templates *template.Template
}

func NewTemplate(filePath string) Querier {
	viewsPath := path.Join(filePath, "views", "**", "*.html")
	return &Templates{
		templates: template.Must(template.ParseGlob(viewsPath)),
	}
}

func wrietWebPage(tmpl *template.Template, w http.ResponseWriter, name string, vars interface{}) error {
	// set cookies && sessions
	err := tmpl.ExecuteTemplate(w, name, vars)
	if err != nil {
		http.Error(w, "failed to load "+err.Error(), http.StatusInternalServerError)
		return err
	}
	return nil
}
