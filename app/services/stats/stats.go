// Package stats maintains the download counters. With Redis configured,
// downloads are counted with INCR and flushed to the database periodically by
// a background flusher; without Redis, every download is written through to
// the database synchronously.
package stats

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/redis/go-redis/v9"

	"github.com/emo-lang/emoji-registry/app/models"
)

const (
	pkgKeyPrefix     = "stats:dl:pkg:"
	versionKeyPrefix = "stats:dl:ver:"
)

var rdb *redis.Client

// Setup switches the service to Redis aggregation mode. Never called (or
// called with nil) means synchronous database writes.
func Setup(client *redis.Client) {
	rdb = client
}

// BackendName reports the active counter store: "redis" or "database".
func BackendName() string {
	if rdb != nil {
		return "redis"
	}
	return "database"
}

// DayCount is one day of a download time series.
type DayCount struct {
	Date  string
	Count int64
}

// Record counts one download of the given package version.
func Record(pkgID, versionID sql.IdType, at time.Time) error {
	if rdb != nil {
		date := at.Format("2006-01-02")
		pipe := rdb.Pipeline()
		pipe.Incr(context.Background(), pkgKey(pkgID))
		pipe.Incr(context.Background(), versionKey(versionID, date))
		_, err := pipe.Exec(context.Background())
		return err
	}

	return recordSync(pkgID, versionID, at)
}

// recordSync writes both counters straight to the database.
func recordSync(pkgID, versionID sql.IdType, at time.Time) error {
	pkg, err := repo.FindByID[models.Package](pkgID)
	if err != nil {
		return err
	}
	if pkg == nil {
		return fmt.Errorf("package %d not found", pkgID)
	}
	if err := repo.UpdateByID[models.Package](pkg.ID, sql.H{
		"downloads":  pkg.Downloads + 1,
		"updated_at": at,
	}); err != nil {
		return err
	}

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

// Flush moves every pending Redis counter into the database and deletes the
// flushed keys. GETDEL makes the read-and-clear atomic per key, so downloads
// arriving during a flush land on a fresh counter and survive.
func Flush() error {
	if rdb == nil {
		return nil
	}

	ctx := context.Background()

	if err := flushKeys(ctx, versionKeyPrefix+"*", func(name string, count int64) error {
		versionID, date, err := parseVersionKey(name)
		if err != nil {
			return err
		}
		return addDownloadCount(versionID, date, count)
	}); err != nil {
		return err
	}

	return flushKeys(ctx, pkgKeyPrefix+"*", func(name string, count int64) error {
		pkgID, err := strconv.ParseInt(strings.TrimPrefix(name, pkgKeyPrefix), 10, 64)
		if err != nil {
			return fmt.Errorf("malformed key %q: %w", name, err)
		}

		pkg, err := repo.FindByID[models.Package](sql.IdType(pkgID))
		if err != nil || pkg == nil {
			return err
		}
		return repo.UpdateByID[models.Package](pkg.ID, sql.H{"downloads": pkg.Downloads + count})
	})
}

// StartFlusher flushes every interval until the process exits. A final flush
// on shutdown is left to the deploy (the counters stay in Redis, so nothing
// is lost when the process stops).
func StartFlusher(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := Flush(); err != nil {
			log.Printf("stats flush failed: %v", err)
		}
	}
}

func flushKeys(ctx context.Context, pattern string, apply func(key string, count int64) error) error {
	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		for _, key := range keys {
			value, err := rdb.GetDel(ctx, key).Result()
			if err == redis.Nil {
				continue
			}
			if err != nil {
				return err
			}

			count, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("malformed counter %q: %w", key, err)
			}
			if err := apply(key, count); err != nil {
				return err
			}
		}

		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func pkgKey(id sql.IdType) string {
	return fmt.Sprintf("%s%d", pkgKeyPrefix, id)
}

func versionKey(id sql.IdType, date string) string {
	return fmt.Sprintf("%s%d:%s", versionKeyPrefix, id, date)
}

func parseVersionKey(key string) (sql.IdType, string, error) {
	rest := strings.TrimPrefix(key, versionKeyPrefix)
	idPart, date, found := strings.Cut(rest, ":")
	if !found {
		return 0, "", fmt.Errorf("malformed key %q", key)
	}

	id, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("malformed key %q: %w", key, err)
	}

	return sql.IdType(id), date, nil
}

func addDownloadCount(versionID sql.IdType, date string, count int64) error {
	row, err := repo.FindOneBy[models.Download](sql.H{"version_id": versionID, "date": date})
	if err != nil {
		return err
	}
	if row == nil {
		_, err = repo.CreateFrom[models.Download](sql.H{
			"version_id": versionID,
			"date":       date,
			"count":      count,
		})
		return err
	}
	return repo.UpdateByID[models.Download](row.ID, sql.H{"count": row.Count + count})
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
