package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/homemade/jgforce"
	"github.com/homemade/jgforce/cmd/worker/justgiving"

	log "github.com/Sirupsen/logrus"
	"github.com/bgentry/que-go"
	"github.com/jackc/pgx"
)

func jgJob(j *que.Job) error {
	err := justgiving.HeartBeat()
	if err != nil {
		log.Errorf("error in justgiving worker %v", err)
	}
	return err
}

func sfJob(j *que.Job) error {
	log.Info("TODO SalesForce worker...")
	return nil
}

func main() {
	var qc *que.Client
	var pgxpool *pgx.ConnPool
	var err error

	dbURL := os.Getenv("DATABASE_URL")
	pgxpool, qc, err = jgforce.Setup(dbURL)
	if err != nil {
		log.WithField("DATABASE_URL", dbURL).Fatal("Error setting up the queue / database: ", err)
	}
	defer pgxpool.Close()

	// Just 1 worker / go routines in each pool (1 for each queue)
	jgWorkers := que.NewWorkerPool(qc, que.WorkMap{
		jgforce.HeartbeatJob: jgJob,
	}, 1)
	jgWorkers.Queue = jgforce.JustGivingQueue
	jgWorkers.Interval = 30 * time.Second // our heartbeat is set in minutes so no point polling too often
	sfWorkers := que.NewWorkerPool(qc, que.WorkMap{
		jgforce.HeartbeatJob: sfJob,
	}, 1)
	sfWorkers.Queue = jgforce.SalesForceQueue
	sfWorkers.Interval = 30 * time.Second // our heartbeat is set in minutes so no point polling too often

	// Catch signal so we can shutdown gracefully
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go jgWorkers.Start()
	go sfWorkers.Start()

	// Wait for a signal
	sig := <-sigCh
	log.WithField("signal", sig).Info("Signal received. Shutting down.")

	justgiving.Shutdown()

	jgWorkers.Shutdown()
	sfWorkers.Shutdown()
}
