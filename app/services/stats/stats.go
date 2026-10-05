// Package stats maintains the per-version, per-day download aggregates.
// Recording is synchronous and read-modify-write; the unique
// (version_id, date) index keeps rows canonical, and a lost race at worst
// undercounts — acceptable until the Redis aggregation of phase 3.
package stats

import (
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/models"
)

// DayCount is one day of a download time series.
type DayCount struct {
	Date  string
	Count int64
}

// Record increments today's counter for the version.
func Record(versionID sql.IdType, at time.Time) error {
	date := at.Format("2006-01-02")

	row, err := repo.FindOneBy[models.Download](sql.H{"version_id": versionID, "date": date})
	if err != nil {
		return err
	}

	if row == nil {
		_, err = repo.CreateFrom[models.Download](sql.H{
			"version_id": versionID,
			"date":       date,
			"count":      1,
		})
		return err
	}

	return repo.UpdateByID[models.Download](row.ID, sql.H{"count": row.Count + 1})
}

// SumSince totals downloads of the given versions on or after the date.
func SumSince(versionIDs []sql.IdType, since string) (int64, error) {
	if len(versionIDs) == 0 {
		return 0, nil
	}

	rows, err := repo.Find[models.Download](repo.CurrentDB(), sql.FindByCond(models.Download{},
		sql.AllOf(sql.In("version_id", versionIDs), sql.Gte("date", since))))
	if err != nil {
		return 0, err
	}

	var total int64
	for _, row := range rows {
		total += row.Count
	}
	return total, nil
}

// DailySeries returns one entry per day for the last days days, zeros
// filled, oldest first.
func DailySeries(versionIDs []sql.IdType, days int, now time.Time) ([]DayCount, error) {
	series := make([]DayCount, 0, days)
	byDate := map[string]int64{}

	for i := days - 1; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		series = append(series, DayCount{Date: date})
		byDate[date] = 0
	}

	if len(versionIDs) == 0 {
		return series, nil
	}

	rows, err := repo.Find[models.Download](repo.CurrentDB(), sql.FindByCond(models.Download{},
		sql.AllOf(sql.In("version_id", versionIDs), sql.Gte("date", series[0].Date))))
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		byDate[row.Date] += row.Count
	}
	for i := range series {
		series[i].Count = byDate[series[i].Date]
	}

	return series, nil
}
