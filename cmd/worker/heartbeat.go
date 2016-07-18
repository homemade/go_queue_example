package main

import (
	"fmt"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/jackc/pgx"
)

func heartbeat() error {
	// connect to salesforce database
	dbURL := os.Getenv("DATABASE_URL")
	connCfg, err := pgx.ParseURI(dbURL)
	if err != nil {
		return fmt.Errorf("error configuring connection to salesforce database %v", err)
	}
	conn, err := pgx.Connect(connCfg)
	if err != nil {
		return fmt.Errorf("error connecting to salesforce database %v", err)
	}
	defer conn.Close()

	// sync data

	// first query salesforce to build a list of records to sync
	_, err := salesforce.Query(conn)
	if err != nil {
		return fmt.Errorf("error querying salesforce records %v", err)
	}

	log.Info("TODO query JG for latest info on fundraising pages...")

	log.Info("TODO update SalesForce database with latest info on fundraising pages...")

	return nil
}
