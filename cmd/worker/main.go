package main

import (
	"os"
	"os/signal"
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

func heartbeatJob(j *que.Job) error {
	return heartbeat()
}

func main() {

	var err error
	dbURL := os.Getenv("DATABASE_URL")
	pgxpool, qc, err = jgforce.Setup(dbURL)
	if err != nil {
		log.WithField("DATABASE_URL", dbURL).Fatal("Errors setting up the queue / database: ", err)
	}
	defer pgxpool.Close()

	wm := que.WorkMap{
		jgforce.HeartbeatJob: heartbeatJob,
	}

	// 1 worker go routine
	workers := que.NewWorkerPool(qc, wm, 1)

	// Catch signal so we can shutdown gracefully
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go workers.Start()

	// Wait for a signal
	sig := <-sigCh
	log.WithField("signal", sig).Info("Signal received. Shutting down.")

	workers.Shutdown()
}
