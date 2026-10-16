package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func renderBase(t *testing.T, opts ...BaseOptions) string {
	t.Helper()

	var buf bytes.Buffer
	ctx := templ.WithChildren(context.Background(), templ.Raw("<p>hi</p>"))
	if err := Base("Page", "alice", false, opts...).Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestBaseHidesTipsSidebarByDefault(t *testing.T) {
	body := renderBase(t)

	if strings.Contains(body, `class="tip-card"`) || strings.Contains(body, `class="er-sidebar"`) {
		t.Fatalf("expected no tips sidebar by default, got %q", body)
	}
	if !strings.Contains(body, "<p>hi</p>") {
		t.Fatalf("expected the page body to render")
	}
}

func TestBaseShowsTipsSidebarWithOption(t *testing.T) {
	body := renderBase(t, BaseOptions{ShowSidebar: true})

	if !strings.Contains(body, `class="tip-card"`) || !strings.Contains(body, `class="er-sidebar"`) {
		t.Fatalf("expected the tips sidebar, got %q", body)
	}
}

func TestBaseEmitsDescriptionWhenSet(t *testing.T) {
	body := renderBase(t, BaseOptions{Description: "A package."})

	if !strings.Contains(body, `<meta name="description" content="A package.">`) {
		t.Fatalf("expected the meta description, got %q", body)
	}
}

// The header carries a light/dark toggle, and a script that applies the
// stored (or OS) theme before the first paint.
func TestBaseRendersThemeToggle(t *testing.T) {
	body := renderBase(t)

	for _, want := range []string{
		`id="theme-toggle"`,
		`class="icon-moon"`,
		`class="icon-sun"`,
		`localStorage.getItem("theme")`,
		`document.documentElement.dataset.theme = theme`,
		`:root[data-theme="dark"]`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q", want)
		}
	}
}
