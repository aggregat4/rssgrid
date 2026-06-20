package templates

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"time"
)

//go:embed *.html
var templateFS embed.FS

//go:embed *.css
var staticFS embed.FS

// LoadTemplates loads all HTML templates from the embedded filesystem
func LoadTemplates() (*template.Template, error) {
	// Create a template set with functions
	funcMap := template.FuncMap{
		"sub": func(a, b int) int {
			return a - b
		},
		"orTime": func(a, b time.Time) time.Time {
			if !a.IsZero() {
				return a
			}
			return b
		},
		"reltime": reltime,
	}

	tmpl := template.New("").Funcs(funcMap)

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

func pluralize(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

// reltime renders a time.Time as a human-friendly relative string such as
// "just now", "5 minutes ago", "3 hours ago", or "2 days ago". A zero time
// renders as "never".
func reltime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < 0:
		return "in the future"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return pluralize(mins, "minute") + " ago"
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return pluralize(hours, "hour") + " ago"
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return pluralize(days, "day") + " ago"
	}
}

// CreateStaticFileServer creates an http.Handler that serves static files from the embedded filesystem
func CreateStaticFileServer() http.Handler {
	return http.FileServer(http.FS(staticFS))
}
