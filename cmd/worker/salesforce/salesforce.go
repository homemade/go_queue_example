package salesforce

import (
	"errors"
	"fmt"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/homemade/justin"
	"github.com/jackc/pgx"
)

func HeartBeat() error {

	// create justin service
	key := os.Getenv("JUSTIN_APIKEY")
	if key == "" {
		return errors.New("missing justin api key")
	}
	ctx := justin.APIKeyContext{
		APIKey:         key,
		Env:            justin.Live,
		Timeout:        (time.Second * 20),
		SkipValidation: true,
	}
	_, err := justin.CreateWithAPIKey(ctx)
	if err != nil {
		return fmt.Errorf("error creating justin service %v", err)
	}

	// connect to justgiving database
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

	// next, retrieve new contacts
	contacts, err := conn.Query("SELECT sfid, jg_charity_id__c, event_id__c, fundraising_page_id__c, fundraising_page_url__c, team_page_url__c, email FROM salesforce.contact;")
	if err != nil {
		return fmt.Errorf("error querying salesforce.contact %v", err)
	}
	defer contacts.Close()
	var crecs []ContactRecord
	for contacts.Next() {
		var r ContactRecord
		if err := contacts.Scan(&r.ID, &r.CharityID, &r.EventID, &r.PageID, &r.PageURL, &r.TeamPageURL, &r.Email); err != nil {
			return fmt.Errorf("error reading from salesforce.contact %v", err)
		}
		if *r.ID != "" {
			crecs = append(crecs, r)
		}
	}

	log.Infof("TODO process %d SalesForce Contact records ...", len(crecs))
	return nil
}

type ContactRecord struct {
	ID          *string
	CharityID   *string
	EventID     *string
	PageID      *string
	PageURL     *string
	TeamPageURL *string
	Email       *string
}
