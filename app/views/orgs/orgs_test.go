package orgs

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/emo-lang/emoji-registry/app/models"
)

func renderComponent(t *testing.T, c templ.Component) string {
	t.Helper()

	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

// The organization pages opt out of the right-hand tips sidebar.
func TestOrgPagesHideTipsSidebar(t *testing.T) {
	pages := map[string]templ.Component{
		"index": Index("alice", false, nil, ""),
		"new":   New("alice", false, "", "", ""),
		"show":  Show("alice", false, &models.Organization{Name: "acme"}, nil, nil, false, ""),
	}

	for name, page := range pages {
		body := renderComponent(t, page)
		if strings.Contains(body, `class="tip-card"`) || strings.Contains(body, `class="er-sidebar"`) {
			t.Fatalf("%s: expected no tips sidebar, got %q", name, body)
		}
	}
}

// The organization page carries the locked badge only while locked.
func TestShowLockedBadge(t *testing.T) {
	body := renderComponent(t, Show("alice", false, &models.Organization{Name: "acme", Locked: true}, nil, nil, false, ""))
	if !strings.Contains(body, "已锁定") {
		t.Fatalf("expected the locked badge, got %q", body)
	}

	body = renderComponent(t, Show("alice", false, &models.Organization{Name: "acme"}, nil, nil, false, ""))
	if strings.Contains(body, "已锁定") {
		t.Fatalf("unlocked organization must not carry the badge, got %q", body)
	}
}
