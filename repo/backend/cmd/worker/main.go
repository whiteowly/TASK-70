package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fieldserve/internal/alerts"
	"fieldserve/internal/analytics"
	"fieldserve/internal/audit"
	"fieldserve/internal/platform/cleanup"

	_ "github.com/lib/pq"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	log.Println("worker started")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			runJobs(db)
		case sig := <-quit:
			log.Printf("received signal %v, shutting down worker", sig)
			return
		}
	}
}

func runJobs(db *sql.DB) {
	ctx := context.Background()
	now := time.Now()

	auditSvc := audit.NewService(db)
	alertsSvc := alerts.NewService(db, auditSvc)

	if err := alertsSvc.EvaluateRules(ctx, now); err != nil {
		log.Printf("worker: evaluate rules error: %v", err)
	} else {
		log.Println("worker: alert rules evaluated")
	}

	if err := alertsSvc.CheckSLADeadlines(ctx, now); err != nil {
		log.Printf("worker: sla check error: %v", err)
	} else {
		log.Println("worker: SLA deadlines checked")
	}

	if err := alertsSvc.EscalateUnacknowledged(ctx, now); err != nil {
		log.Printf("worker: escalation error: %v", err)
	} else {
		log.Println("worker: unacknowledged alert escalation checked")
	}

	today := now.Format("2006-01-02")
	if err := analytics.RunDailyRollups(db, today); err != nil {
		log.Printf("worker: rollup error: %v", err)
	} else {
		log.Println("worker: daily rollups checked/generated")
	}

	// Cleanup expired data using shared cleanup functions
	if n, err := cleanup.ExpiredSessions(db); err != nil {
		log.Printf("worker: session cleanup error: %v", err)
	} else {
		log.Printf("worker: expired sessions cleaned (%d removed)", n)
	}

	if n, err := cleanup.ExpiredIdempotencyKeys(db); err != nil {
		log.Printf("worker: idempotency cleanup error: %v", err)
	} else {
		log.Printf("worker: expired idempotency keys cleaned (%d removed)", n)
	}

	if n, err := cleanup.ExpiredEvidence(db); err != nil {
		log.Printf("worker: evidence cleanup error: %v", err)
	} else {
		log.Printf("worker: expired evidence cleaned (%d removed)", n)
	}
}
