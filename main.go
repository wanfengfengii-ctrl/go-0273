// Command strawberry-vitro-acclimation-gate serves the facility strawberry
// vitrified-shootlet acclimation gate HTTP backend. It opens and migrates the
// SQLite database, recovers open tasks and pending device calls from the last
// run, wires the catalog/task/sample/measure/pathogen services, and serves the
// JSON HTTP API.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"strawberry-vitro-acclimation-gate/catalog"
	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/httpapi"
	"strawberry-vitro-acclimation-gate/measure"
	"strawberry-vitro-acclimation-gate/pathogen"
	"strawberry-vitro-acclimation-gate/sample"
	"strawberry-vitro-acclimation-gate/store"
	"strawberry-vitro-acclimation-gate/task"
)

func main() {
	dbPath := os.Getenv("BENZHI_DB_PATH")
	if dbPath == "" {
		dbPath = "benzhi.db"
	}
	addr := os.Getenv("BENZHI_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer db.Close()

	clock := domain.NewLogicalClock(0)
	ctx := context.Background()
	report, err := db.Recover(ctx, clock.Now())
	if err != nil {
		log.Fatalf("recover: %v", err)
	}
	log.Printf("recovered: open_tasks=%d active_leases=%d pending_calls=%d revealed_codes=%d permits=%d",
		report.OpenTasks, report.ActiveLeases, report.PendingCalls, report.RevealedCodes, report.Permits)

	catalogSvc := catalog.NewService(db)
	taskSvc := task.NewService(db, db, clock)
	sampleSvc := sample.NewService(db, clock)
	measureSvc := measure.NewService(db, clock)
	pathogenSvc := pathogen.NewService(db, clock)

	ready := func() bool {
		if err := db.PingContext(ctx); err != nil {
			return false
		}
		return true
	}

	srv := httpapi.NewServer(catalogSvc, taskSvc, sampleSvc, measureSvc, pathogenSvc, ready)
	log.Printf("listening on %s (db=%s)", addr, dbPath)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatal(err)
	}
}
