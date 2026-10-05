package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
)

// Render converts Markdown to HTML with goldmark's default renderer, which
// escapes raw HTML in the source (html.WithUnsafe stays off), so README
// content cannot inject markup or scripts into the package page.
func Render(source []byte) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert(source, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
