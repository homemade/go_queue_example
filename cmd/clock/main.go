package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	que "github.com/bgentry/que-go"

	"github.com/homemade/jgforce"


)

func main() {

	// Setup queue / database
	dbURL := os.Getenv("DATABASE_URL")
	pgxpool, qc, err := jgforce.Setup(dbURL)
	if err != nil {
		log.WithField("DATABASE_URL", dbURL).Fatal("Unable to setup queue / database")
	}

	// Catch signals so we can shutdown gracefully
	// (Heroku will cycle processes daily sending a SIGTERM)
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Kick off timer
	ticker := time.NewTicker(time.Second * 60)
	go func() {
		for t := range ticker.C {
			log.WithField("tick", t).Info("Queuing heartbeat event")
			// Queue the heartbeat event
			j := que.Job{
				Type: jgforce.HeartbeatJob,
			}
			if err := qc.Enqueue(&j); err != nil {
				log.Error(fmt.Errorf("Unable to queue heartbeat event %v",err))
			}
		}

	}()

	// Wait for signals and handle them gracefully by closing the postgres connection pool and stopping the ticker
	sig := <-sigCh
	log.WithField("signal", sig).Info("Signal received. Shutting down.")
	pgxpool.Close()
	ticker.Stop()
}
