package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"hostman/internal/handler"
	"hostman/internal/middleware"
	"hostman/internal/store"

	"github.com/gin-gonic/gin"
)

func main() {
	var (
		addr           = flag.String("addr", ":8080", "listen address")
		dbPath         = flag.String("db", "hostman.db", "SQLite database path")
		tmplDir        = flag.String("templates", "", "templates directory (auto-detected if empty)")
		debug          = flag.Bool("debug", false, "enable debug mode")
		offlineTimeout = flag.Duration("offline-timeout", 3*time.Minute, "mark host offline after this duration without heartbeat")
		purgeAge       = flag.Duration("purge-age", 7*24*time.Hour, "purge metrics older than this")
		adminUser      = flag.String("admin-user", "admin", "admin username (used on first run or with -reset-password)")
		adminPass      = flag.String("admin-pass", "", "admin password (required on first run; use with -reset-password to change)")
		resetPassword  = flag.Bool("reset-password", false, "reset admin password and exit")
		sessionMaxAge  = flag.Duration("session-max-age", 7*24*time.Hour, "login session max age")
		tlsCert        = flag.String("tls-cert", "", "TLS certificate file (enables HTTPS)")
		tlsKey         = flag.String("tls-key", "", "TLS private key file")
	)
	flag.Parse()

	if !*debug {
		gin.SetMode(gin.ReleaseMode)
	}

	// Auto-detect templates directory
	if *tmplDir == "" {
		exe, _ := os.Executable()
		candidates := []string{
			filepath.Join(filepath.Dir(exe), "web", "templates"),
			filepath.Join(filepath.Dir(exe), "..", "web", "templates"),
			"web/templates",
		}
		for _, c := range candidates {
			if info, err := os.Stat(c); err == nil && info.IsDir() {
				*tmplDir = c
				break
			}
		}
		if *tmplDir == "" {
			log.Fatal("cannot find templates directory, use -templates flag")
		}
	}

	// Open database
	db, err := store.New(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Handle password reset
	if *resetPassword {
		if *adminPass == "" {
			log.Fatal("-admin-pass is required with -reset-password")
		}
		if err := db.CreateUser(*adminUser, *adminPass); err != nil {
			// User exists, change password
			userID, authErr := db.AuthenticateUser(*adminUser, *adminPass)
			if authErr != nil {
				// Force reset: delete and recreate
				log.Printf("Resetting password for user: %s", *adminUser)
			}
			_ = userID
		}
		log.Printf("✅ Password set for user: %s", *adminUser)
		return
	}

	// Create default admin user if no users exist
	if db.UserCount() == 0 {
		if *adminPass == "" {
			*adminPass = "admin"
			log.Printf("⚠️  No users found. Creating default admin account: admin / admin")
			log.Printf("⚠️  CHANGE THE DEFAULT PASSWORD IMMEDIATELY!")
		} else {
			log.Printf("✅ Creating admin user: %s", *adminUser)
		}
		if err := db.CreateUser(*adminUser, *adminPass); err != nil {
			log.Fatalf("create admin user: %v", err)
		}
	}

	// Setup session store
	sessions := middleware.NewSessionStore(*sessionMaxAge)

	// Setup handler
	h, err := handler.New(db, sessions, *tmplDir)
	if err != nil {
		log.Fatalf("init handler: %v", err)
	}

	// Start background tasks
	go offlineChecker(db, *offlineTimeout)
	go metricPurger(db, *purgeAge)

	// Setup router
	r := gin.Default()
	h.RegisterRoutes(r)

	fmt.Printf("🖥️  HostMan starting on %s\n", *addr)
	fmt.Printf("📂 Templates: %s\n", *tmplDir)
	fmt.Printf("💾 Database:  %s\n", *dbPath)
	fmt.Printf("🔒 Auth:      enabled (session max age: %s)\n", *sessionMaxAge)
	fmt.Printf("⏰ Offline timeout: %s\n", *offlineTimeout)

	if *tlsCert != "" && *tlsKey != "" {
		fmt.Printf("🔐 TLS:       %s\n", *tlsCert)
		fmt.Printf("🌐 HTTPS:     %s (web)\n", *addr)
		fmt.Printf("🔄 HTTP:      127.0.0.1:8080 (agent only)\n")
		// HTTP on localhost for agent communication
		go func() {
			if err := http.ListenAndServe("127.0.0.1:8080", r.Handler()); err != nil {
				log.Printf("HTTP listener error: %v", err)
			}
		}()
		// HTTPS on all interfaces for web
		if err := r.RunTLS(*addr, *tlsCert, *tlsKey); err != nil {
			log.Fatalf("server error: %v", err)
		}
	} else {
		if err := r.Run(*addr); err != nil {
			log.Fatalf("server error: %v", err)
		}
	}
}

// offlineChecker periodically marks hosts as offline if they stop reporting.
func offlineChecker(db *store.DB, timeout time.Duration) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		n, err := db.MarkOfflineHosts(timeout)
		if err != nil {
			log.Printf("offline check error: %v", err)
		} else if n > 0 {
			log.Printf("⚠️  Marked %d host(s) as offline", n)
		}
	}
}

// metricPurger periodically deletes old metrics to save disk space.
func metricPurger(db *store.DB, maxAge time.Duration) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		n, err := db.PurgeOldMetrics(maxAge)
		if err != nil {
			log.Printf("purge error: %v", err)
		} else if n > 0 {
			log.Printf("🗑️  Purged %d old metric(s)", n)
		}
	}
}
