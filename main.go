package main

import (
	"errors"
	"log"
	"os"
	"strings"
	"time"

	airwayredis "github.com/daqing/airway-redis-plugin"
	"github.com/daqing/airway/app/websocket"
	"github.com/daqing/airway/cmd"
	"github.com/daqing/airway/lib/app"
	"github.com/daqing/airway/lib/jsbuild"
	"github.com/daqing/airway/lib/plugin"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/storage"
	"github.com/daqing/airway/lib/utils"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
	"github.com/emo-lang/emoji-registry/app/services/export"
	"github.com/emo-lang/emoji-registry/app/services/stats"
	"github.com/emo-lang/emoji-registry/config"

	// Registers the Go DSL migrations under db/migrate with
	// lib/migrate/schema (the package ships a doc.go placeholder until
	// `generate migration` adds files).
	_ "github.com/emo-lang/emoji-registry/db/migrate"
)

// The project binary starts the HTTP server by default (or via `server`).
// Any other argument is dispatched to the Airway CLI compiled into this
// binary, so project-local code (REPL models, plugins, Go DSL migrations
// imported below) is visible to commands like `go run . repl`.
func main() {
	args := os.Args[1:]

	// `--version` / `-v` print the VERSION file contents directly, without
	// loading .env or any other project setup.
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-v") {
		printVersion()
		return
	}

	if len(args) == 0 || args[0] == "server" {
		runServer()
		return
	}

	// Project-local command: export the registry as a static protocol A file
	// tree. Intercepted before the Airway CLI, following the --version
	// precedent.
	if args[0] == "registry:export" {
		runRegistryExport(args[1:])
		return
	}

	// `admin:grant <username>` promotes a user to admin. The first admin can
	// only be created here — there is deliberately no UI for it.
	if args[0] == "admin:grant" {
		runAdminGrant(args[1:])
		return
	}

	cmd.Version = versionString()
	loadCLIEnv()
	cmd.Run(args)
}

// runAdminGrant sets admin=true for the named user.
func runAdminGrant(args []string) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		log.Println("usage: admin:grant <username>")
		os.Exit(1)
	}
	username := strings.TrimSpace(args[0])

	loadCLIEnv()

	dsn := utils.GetEnvMulti("AIRWAY_DSN", "DSN")
	if len(dsn) == 0 {
		log.Println("DSN is not set")
		os.Exit(1)
	}
	if _, err := repo.SetupDB(dsn); err != nil {
		log.Printf("database setup failed: %v", err)
		os.Exit(3)
	}

	user, err := repo.FindOneBy[models.User](sql.H{"username": username})
	if err != nil {
		log.Printf("lookup failed: %v", err)
		os.Exit(8)
	}
	if user == nil {
		log.Printf("no such user: %s", username)
		os.Exit(8)
	}

	if err := repo.UpdateByID[models.User](user.ID, sql.H{"admin": true}); err != nil {
		log.Printf("grant failed: %v", err)
		os.Exit(8)
	}

	log.Printf("%s is now an admin", username)
}

// runRegistryExport wires the same infrastructure as the server (env, DB,
// storage) and exports the registry into a directory.
func runRegistryExport(args []string) {
	dir := "./dist-registry"
	if len(args) > 0 {
		dir = args[0]
	}

	loadCLIEnv()

	dsn := utils.GetEnvMulti("AIRWAY_DSN", "DSN")
	if len(dsn) == 0 {
		log.Println("DSN is not set")
		os.Exit(1)
	}
	if _, err := repo.SetupDB(dsn); err != nil {
		log.Printf("database setup failed: %v", err)
		os.Exit(3)
	}
	if _, err := storage.Setup(storage.FromEnv()); err != nil {
		log.Printf("storage setup failed: %v", err)
		os.Exit(4)
	}

	stats, err := export.Run(dir)
	if err != nil {
		log.Printf("export failed: %v", err)
		os.Exit(7)
	}

	log.Printf("exported %d packages (%d versions, %d source files, %d archives) to %s; skipped %d private packages",
		stats.Packages, stats.Versions, stats.Files, stats.Archives, dir, stats.SkippedPrivate)
}

func runServer() {
	// .env supplies fallbacks for anything the process environment does not
	// set (godotenv.Load never overrides existing values), so
	// `LISTEN=0.0.0.0:1988 airway server` wins over a LISTEN in .env. Load it
	// before any env checks so AIRWAY_ENV itself can come from .env.
	if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("Loading env file: .env failed: %v", err)
	}

	appConfig := utils.AppConfig()

	if appConfig.Env == "" {
		log.Println("AIRWAY_ENV is not set")
		os.Exit(1)
	}

	if !appConfig.IsLocal {
		gin.SetMode(gin.ReleaseMode)
	}

	dsn := utils.GetEnvMulti("AIRWAY_DSN", "DSN")

	if len(dsn) > 0 {
		if _, setupErr := repo.SetupDB(dsn); setupErr != nil {
			log.Printf("database setup failed: %v", setupErr)
			os.Exit(3)
		}

		// The reserved_names table is seeded from the hardcoded stdlib list;
		// seeding is idempotent, so it runs on every boot.
		if err := emoji.SeedReservedNames(); err != nil {
			log.Printf("reserved names seed failed: %v", err)
			os.Exit(3)
		}
	}

	// Redis is optional: when REDIS is configured and reachable, rate limiting
	// switches to Redis-backed counters and download stats aggregate in Redis
	// with a periodic flush to the database.
	redisCfg := airwayredis.FromEnv()
	if redisCfg.Enabled() {
		if err := airwayredis.Setup(redisCfg); err != nil {
			log.Printf("redis unavailable, falling back to in-process backends: %v", err)
		}
	}
	if airwayredis.Current() != nil {
		stats.Setup(airwayredis.Current())
		go stats.StartFlusher(time.Minute)
	}
	log.Printf("rate limit backend: %s; download stats: %s",
		middlewares.RateLimitBackendName(), stats.BackendName())

	// In local development the frontend bundle is rebuilt in memory and
	// served with livereload; production serves the embedded dist bundle.
	// A missing vendor directory aborts the boot: the source watcher skips
	// vendor/, so a running server would never pick up a later js:install.
	if appConfig.IsLocal {
		if _, err := jsbuild.StartDefault(".", websocket.Broadcast); err != nil {
			if errors.Is(err, jsbuild.ErrVendorMissing) {
				log.Printf("frontend dev server failed: %v", err)
				os.Exit(6)
			}
			log.Printf("frontend dev server disabled: %v", err)
		}
	}

	if _, err := storage.Setup(storage.FromEnv()); err != nil {
		log.Printf("storage setup failed: %v", err)
		os.Exit(4)
	}

	if err := plugin.BootAll(); err != nil {
		log.Printf("plugin boot failed: %v", err)
		os.Exit(5)
	}

	runApp()
}

func loadCLIEnv() {
	err := godotenv.Load(".env")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("Loading env file: .env failed: %v", err)
	}
}

func runApp() {
	a := app.NewApp("Airway", app.WithRoutes(config.Routes, config.HealthRoutes))
	a.Run()
}
