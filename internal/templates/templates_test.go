package templates

import (
	"bytes"
	"testing"
	"time"
)

func TestReltime(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", time.Time{}, "never"},
		{"just now", time.Now().Add(-5 * time.Second), "just now"},
		{"one minute", time.Now().Add(-1 * time.Minute), "1 minute ago"},
		{"minutes", time.Now().Add(-5 * time.Minute), "5 minutes ago"},
		{"one hour", time.Now().Add(-1 * time.Hour), "1 hour ago"},
		{"hours", time.Now().Add(-3 * time.Hour), "3 hours ago"},
		{"one day", time.Now().Add(-24 * time.Hour), "1 day ago"},
		{"days", time.Now().Add(-72 * time.Hour), "3 days ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reltime(tt.t); got != tt.want {
				t.Errorf("reltime() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSettingsTemplate_RendersFeedHealthBadge(t *testing.T) {
	tmpl, err := LoadTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	settings := tmpl.Lookup("settings.html")
	if settings == nil {
		t.Fatal("settings.html template not found")
	}

	type feedLike struct {
		ID                  int64
		Title               string
		URL                 string
		ConsecutiveFailures int
		LastError           string
		LastErrorAt         time.Time
		LastSuccessAt       time.Time
		LastFetchedAt       time.Time
	}

	data := struct {
		Feeds         []feedLike
		FlashMessages []struct{ Type, Message string }
		PostsPerFeed  int
		Columns       int
	}{
		Feeds: []feedLike{
			{ID: 1, Title: "Healthy Feed", URL: "https://example.com/healthy.xml"},
			{ID: 2, Title: "Broken Feed", URL: "https://example.com/broken.xml", ConsecutiveFailures: 3, LastError: "connection refused", LastErrorAt: time.Now()},
		},
		PostsPerFeed: 10,
		Columns:      2,
	}

	var buf bytes.Buffer
	if err := settings.Execute(&buf, data); err != nil {
		t.Fatalf("Failed to execute settings template: %v", err)
	}

	out := buf.String()
	if !contains(out, "Failing: connection refused") {
		t.Errorf("expected output to surface the failing feed's error, got:\n%s", out)
	}
	if !contains(out, "(3x)") {
		t.Errorf("expected output to show consecutive failure count (3x), got:\n%s", out)
	}
	if !contains(out, "Last fetched:") {
		t.Errorf("expected output to show a 'Last fetched' line, got:\n%s", out)
	}
	// The healthy feed must not render a failure badge.
	// A simple sanity check: the failure count text appears exactly once.
	if count := occurrences(out, "Failing:"); count != 1 {
		t.Errorf("expected exactly one failing feed rendered, got %d", count)
	}
}

func contains(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub))
}

func occurrences(s, sub string) int {
	var n int
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
			i += len(sub) - 1
		}
	}
	return n
}
