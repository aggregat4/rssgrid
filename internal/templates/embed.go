package templates

import (
	"embed"
	"html/template"
	"io/fs"
)

//go:embed *.html
var templateFS embed.FS

// LoadTemplates loads all HTML templates from the embedded filesystem
func LoadTemplates() (*template.Template, error) {
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

			// Parse the template
			_, err = tmpl.Parse(string(content))
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
