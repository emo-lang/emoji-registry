package main

import (
	"github.com/daqing/airway/cmd"
	"github.com/daqing/airway/lib/static"

	"github.com/emo-lang/emoji-registry/app/views/home"
)

// Static export: `airway static:build` renders the home page shell into
// dist/ together with the committed frontend bundle. The home page is
// data-driven, so the static copy is exported with empty package lists.
func init() {
	cmd.SetStaticPages(
		static.Page{Slug: "/", Component: home.Index("", false, nil, nil)},
	)
}
