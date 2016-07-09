package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
)

func main() {

	// Catch signals so we can shutdown gracefully
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Kick of timer
	ticker := time.NewTicker(time.Second * 60)
	go func() {
		for t := range ticker.C {
			log.WithField("tick", t).Info("TODO handle tick")
		}
	}()

	// Wait for signals
	sig := <-sigCh
	log.WithField("signal", sig).Info("Signal received. Shutting down.")
	ticker.Stop()
}
