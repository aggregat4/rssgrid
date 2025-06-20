package templates

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed *.html
var templateFS embed.FS

//go:embed *.css
var staticFS embed.FS

// LoadTemplates loads all HTML templates from the embedded filesystem
func LoadTemplates() (*template.Template, error) {
	// Create a template set with a base template
	tmpl := template.New("")

	// Walk through all .html files in the embedded filesystem
	err := fs.WalkDir(templateFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			// Read the template file
			content, err := templateFS.ReadFile(path)
			if err != nil {
				return err
			}

			// Parse the template and add it to the template set
			// Use the filename as the template name
			_, err = tmpl.New(path).Parse(string(content))
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return tmpl, nil
}

// CreateStaticFileServer creates an http.Handler that serves static files from the embedded filesystem
func CreateStaticFileServer() http.Handler {
	return http.FileServer(http.FS(staticFS))
}
