package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	que "github.com/bgentry/que-go"

	"github.com/homemade/jgforce"
)

func main() {

	// read heartbeat
	htbt, err := strconv.Atoi(os.Getenv("HEARTBEAT"))
	if htbt < 1 || err != nil {
		log.WithField("HEARTBEAT", htbt).Fatal(fmt.Sprintf("Unable to setup heartbeat %v", err))
	}

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
	ticker := time.NewTicker(time.Minute * time.Duration(htbt))
	go func() {
		for t := range ticker.C {
			// add heartbeat event to our 2 queues
			log.WithField("tick", t).Info(fmt.Sprintf("Adding heartbeat event to queue %s", jgforce.JustGivingQueue))
			j1 := que.Job{
				Queue: jgforce.JustGivingQueue,
				Type:  jgforce.HeartbeatJob,
			}
			if err := qc.Enqueue(&j1); err != nil {
				log.Error(fmt.Errorf("Unable to add heartbeat event to queue %s, error %v", jgforce.JustGivingQueue, err))
			}
			log.WithField("tick", t).Info(fmt.Sprintf("Adding heartbeat event to queue %s", jgforce.SalesForceQueue))
			j2 := que.Job{
				Queue: jgforce.SalesForceQueue,
				Type:  jgforce.HeartbeatJob,
			}
			if err := qc.Enqueue(&j2); err != nil {
				log.Error(fmt.Errorf("Unable to add heartbeat event to queue %s, error %v", jgforce.SalesForceQueue, err))
			}
		}

	}()

	// Wait for signals and handle them gracefully by closing the postgres connection pool and stopping the ticker
	sig := <-sigCh
	log.WithField("signal", sig).Info("Signal received. Shutting down.")
	pgxpool.Close()
	ticker.Stop()
}
