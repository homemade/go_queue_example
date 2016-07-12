package main

import (
	"database/sql"
	"os"

	log "github.com/Sirupsen/logrus"
	_ "github.com/lib/pq"
)

func heartbeat() error {

	querySalesForce()

	log.Info("TODO query JG for latest info on fundraising pages...")

	log.Info("TODO update SalesForce database with latest info on fundraising pages...")

	return nil
}

func querySalesForce() {
	dbURL := os.Getenv("DATABASE_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.WithField("DATABASE_URL", dbURL).Fatal("Errors connecting to salesforce database: ", err)
		return
	}
	rows, err := db.Query("SELECT charityid__c,eventid__c,pageshortname__c,pageid__c,pageemail__c FROM salesforce.jgpage__c")
	if err != nil {
		log.WithField("DATABASE_URL", dbURL).Fatal("Errors reading fundraising pages from salesforce database: ", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var charityID string
		var eventID string
		var pageShortName string
		var pageID string
		var pageEmail string
		if err := rows.Scan(&charityID, &eventID, &pageShortName, &pageID, &pageEmail); err != nil {
			log.WithField("DATABASE_URL", dbURL).Fatal("Errors scanning fundraising pages from salesforce database: ", err)
			return
		}

		log.WithField("charityID", charityID).WithField("eventID", eventID).WithField("pageShortName", pageShortName).WithField("pageID", pageID).WithField("pageEmail", pageEmail).Info("TODO convert salesforce data for JG api call ...")
	}
}
