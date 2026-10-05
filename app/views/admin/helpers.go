package admin

import (
	"strconv"

	airwaysql "github.com/daqing/airway/lib/sql"
)

func idString(id airwaysql.IdType) string {
	return strconv.FormatInt(int64(id), 10)
}
