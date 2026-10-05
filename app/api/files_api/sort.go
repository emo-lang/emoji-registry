package files_api

import (
	"sort"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
)

func sortVersions(versions []*models.Version) {
	sort.Slice(versions, func(i, j int) bool {
		return emoji.CompareVersions(versions[i].Version, versions[j].Version) < 0
	})
}
