package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/homemade/jgforce"

	log "github.com/Sirupsen/logrus"
	"github.com/bgentry/que-go"
	"github.com/jackc/pgx"
)

var (
	qc      *que.Client
	pgxpool *pgx.ConnPool
)

// indexURLJob would do whatever indexing is necessary in the background
func indexURLJob(j *que.Job) error {
	var ir jgforce.IndexRequest
	err := json.Unmarshal(j.Args, &ir)
	if err != nil {
		log.WithField("args", string(j.Args)).Error("Unable to unmarshal job arguments into IndexRequest")
		return err
	}

	log.WithField("IndexRequest", ir).Info("Processing IndexRequest! (not really)")
	// You would do real work here...

	return nil
}

func main() {

	goVer := os.Getenv("GOVERSION")
	if goVer != runtime.Version() {
		log.Fatal("Incompatible Go version detected or GOVERSION not set", fmt.Errorf("Go version is %s but GOVERSION is %s", runtime.Version(), goVer))
	}

	var err error
	dbURL := os.Getenv("DATABASE_URL")
	pgxpool, qc, err = jgforce.Setup(dbURL)
	if err != nil {
		log.WithField("DATABASE_URL", dbURL).Fatal("Errors setting up the queue / database: ", err)
	}
	defer pgxpool.Close()

	wm := que.WorkMap{
		jgforce.IndexRequestJob: indexURLJob,
	}

	// 2 worker go routines
	workers := que.NewWorkerPool(qc, wm, 2)

	// Catch signal so we can shutdown gracefully
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go workers.Start()

	// Wait for a signal
	sig := <-sigCh
	log.WithField("signal", sig).Info("Signal received. Shutting down.")

	workers.Shutdown()
}
