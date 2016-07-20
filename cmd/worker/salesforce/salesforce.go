package salesforce

import (
	"errors"
	"fmt"
	"net/mail"
	"os"
	"strconv"
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
	svc, err := justin.CreateWithAPIKey(ctx)
	if err != nil {
		return fmt.Errorf("error creating justin service %v", err)
	}

	// connect to database
	dbURL := os.Getenv("DATABASE_URL")
	connCfg, err := pgx.ParseURI(dbURL)
	if err != nil {
		return fmt.Errorf("error configuring connection to database %v", err)
	}
	conn, err := pgx.Connect(connCfg)
	if err != nil {
		return fmt.Errorf("error connecting to database %v", err)
	}
	defer conn.Close()

	// next, retrieve new contacts
	sql := `SELECT c.sfid, c.jg_charity_id__c, c.event_id__c, c.fundraising_page_id__c, c.fundraising_page_url__c, c.team_page_url__c, c.email
 FROM salesforce.contact c LEFT OUTER JOIN salesforce.donation_stats__c d
 ON (c.sfid = d.related_contact_record__c)
 WHERE d.sfid IS NULL ORDER BY c.systemmodstamp DESC;`
	contacts, err := conn.Query(sql)
	if err != nil {
		return fmt.Errorf("error querying new salesforce.contacts %v", err)
	}

	var crecs []ContactRecord
	for contacts.Next() {
		var r ContactRecord
		if err := contacts.Scan(&r.ID, &r.CharityID, &r.EventID, &r.PageID, &r.PageURL, &r.TeamPageURL, &r.Email); err != nil {
			return fmt.Errorf("error reading from new salesforce.contacts %v", err)
		}
		if *r.ID != "" {
			crecs = append(crecs, r)
		}
	}
	contacts.Close()

	// try and find a justgiving fundraising page for the new contacts
	for _, c := range crecs {
		// our order of precedence is:
		// 1. Try id first
		charityID := uint(0)
		eventID := uint(0)
		pageID := uint(0)
		// TODO pending data import
		found, err := searchForPageUsingID(svc, conn, charityID, eventID, pageID, c.ID)
		if err != nil {
			return err
		}
		if !found {
			// 2. If we couldn't find a match with the id, try the short name
			shortName := ""
			// TODO pending data import
			found, err = searchForPageUsingShortName(svc, conn, shortName)
			if err != nil {
				return err
			}
			if !found {
				// 3. Finally as a fallback try and use the email address of the contact
				// TODO maybe we should ony do this when we have a valid c.EventID
				if c.Email != nil {
					_, err = searchForPageUsingEmail(svc, conn, c)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	// NOTE: the search functions handle creation of donation stats master records when a matching page is found

	// TODO update donation stats detail records

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

func searchForPageUsingID(svc *justin.Service, conn *pgx.Conn, charityID uint, eventID uint, pageID uint, contactID *string) (bool, error) {
	if pageID == 0 {
		return false, nil
	}
	// look for a match in the justgiving database
	sql := `SELECT page_id FROM justgiving.page_priority WHERE page_id=$1`
	res := 0
	err := conn.QueryRow(sql, pageID).Scan(&res)
	if err == nil && uint(res) == pageID {
		// if there is a match bump the page priority so we refresh its results more often
		// and create a donation stats master record
		return true, handleMatch(conn, pageID, contactID)
	}
	if err == pgx.ErrNoRows {
		// if there is no match, if we have a charity id and event id
		// check event id is in the database and if not add it
		// (hopefully we will then find a match later - once the events pages are retrieved)
		if charityID > 0 && eventID > 0 {
			eventIDs, err := eventIDs(conn)
			if err != nil {
				return false, err
			}
			if !in(eventIDs, eventID) {
				err = addEvent(svc, conn, charityID, eventID, -1) // -1 signifies default priority
				if err != nil {
					return false, err
				}
			}
		}

	}

	return false, nil
}

func searchForPageUsingShortName(svc *justin.Service, conn *pgx.Conn, shortName string) (bool, error) {
	// TODO
	// look for a match in our database using short name
	//
	// if there is a match bump the page priority so we refresh its results more often
	// and create a donation stats master record
	//
	// if there is no match try and retrieve the page via the justgiving api
	// and check event id is in the database and if not add it
	// (hopefully we will then find a match later - once the events pages are retrieved)

	return false, nil
}

func searchForPageUsingEmail(svc *justin.Service, conn *pgx.Conn, c ContactRecord) (bool, error) {
	eml := ""
	if c.Email != nil {
		eml = *c.Email
	}
	if eml == "" {
		log.Warn("missing email in call to searchForPageUsingEmail")
		return false, nil
	}
	account, err := mail.ParseAddress(eml)
	if err != nil {
		log.Warnf("failed to parse email address %s in call to searchForPageUsingEmail %v", eml, err)
		return false, nil
	}
	// try and use default charity id if none is provided
	charityID := 0
	if c.CharityID == nil || *c.CharityID == "" {
		charityID, err = strconv.Atoi(os.Getenv("JUSTIN_CHARITY"))
		if err != nil {
			log.Warnf("failed to set charity id from default %s %v", os.Getenv("JUSTIN_CHARITY"), err)
			return false, nil
		}
	}
	if charityID == 0 {
		log.Warn("missing charity id in call to searchForPageUsingEmail")
		return false, nil
	}

	fprs, err := svc.FundraisingPagesForCharityAndUser(uint(charityID), *account)

	if err != nil {
		return false, err
	}
	// if we find pages, look for a matching event in our database
	if len(fprs) > 0 {

		// fetch events
		events, err := eventIDs(conn)
		if err != nil {
			return false, err
		}
		// check pages, if we find a single matching page which is active call SearchForPageUsingPageID
		matchedIndex := 0
		matchedCount := 0
		for i, p := range fprs {
			if in(events, p.EventID()) {
				// TODO check page is active (has donations) through call to svc.FundraisingPageResults(p)
				matchedIndex = i
				matchedCount = matchedCount + 1
			}
		}
		if matchedCount == 1 {
			p := fprs[matchedIndex]
			return searchForPageUsingID(svc, conn, p.CharityID(), p.EventID(), p.ID(), c.ID)
		}

		// if we didn't match the event and it has an event id above our min event id
		// add the events to our database with a priority of 0 until they can be verified
		// TODO if possible automate this verification step
		// TODO use event start date instead of min event id check
		// TODO maybe we should ony do this where p.EventID == c.EventID
		if matchedCount < 1 {
			for _, p := range fprs {
				// refresh events
				events, err = eventIDs(conn)
				if err != nil {
					return false, err
				}
				if !in(events, p.EventID()) && p.EventID() > 3000000 { // TODO read min event id from env var or maybe query database to retrieve min based on existing events
					addEvent(svc, conn, p.CharityID(), p.EventID(), 0)
					if err != nil {
						return false, err
					}
				}
			}
		}
	}

	return false, nil
}

func eventIDs(conn *pgx.Conn) ([]uint, error) {
	rows, err := conn.Query("SELECT event_id FROM justgiving.event WHERE priority <> 0 ORDER BY priority;")
	if err != nil {
		return nil, fmt.Errorf("error querying justgiving.event %v", err)
	}
	defer rows.Close()
	var events []uint
	for rows.Next() {
		var eventID uint
		if err = rows.Scan(&eventID); err != nil {
			return nil, fmt.Errorf("error reading from justgiving.event %v", err)
		}
		if eventID > 0 {
			events = append(events, eventID)
		}
	}
	return events, nil
}

func in(ids []uint, search uint) bool {
	for _, id := range ids {
		if search == id {
			return true
		}
	}
	return false
}

func addEvent(svc *justin.Service, conn *pgx.Conn, charityID uint, eventID uint, priority int) error {
	// retrieve event from justgiving api
	event, err := svc.Event(eventID)
	if err != nil {
		return fmt.Errorf("error fetching event %d from justgiving %v", eventID, err)
	}
	eventCompletionDate, err := event.ParseCompletionDate()
	if err != nil {
		return fmt.Errorf("error parsing event completion date %s as returned from from justgiving %v", event.CompletionDate, err)
	}
	eventExpiryDate, err := event.ParseExpiryDate()
	if err != nil {
		return fmt.Errorf("error parsing event expiry date %s as returned from from justgiving %v", event.ExpiryDate, err)
	}
	eventStartDate, err := event.ParseStartDate()
	if err != nil {
		return fmt.Errorf("error parsing event start date %s as returned from from justgiving %v", event.StartDate, err)
	}

	sql := `INSERT INTO justgiving.event (charity_id, event_id, name, event_type, location, completion_date, expiry_date, start_date) VALUES($1,$2,$3,$4,$5,$6,$7,$8);`
	if priority >= 0 { // a priority of -1 uses default priority - see sql above
		sql = `INSERT INTO justgiving.event (charity_id, event_id, priority, name, event_type, location, completion_date, expiry_date, start_date) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);`
		_, err = conn.Exec(sql, charityID, eventID, priority, event.Name, event.Type, event.Location, eventCompletionDate, eventExpiryDate, eventStartDate)
	} else {
		_, err = conn.Exec(sql, charityID, eventID, event.Name, event.Type, event.Location, eventCompletionDate, eventExpiryDate, eventStartDate)
	}
	if err != nil {
		return fmt.Errorf("error inserting justgiving.event %d %d %d %v", charityID, eventID, priority, err)
	}
	return nil
}

func handleMatch(conn *pgx.Conn, pageID uint, contactID *string) error {
	// bump the page priority so we refresh its results more often
	sql := `UPDATE justgiving.page_priority SET priority=5 WHERE page_id=$1`
	_, err := conn.Exec(sql, pageID)
	if err != nil {
		return fmt.Errorf("error updating justgiving.page_priority to 5 for page id %d %v", pageID, err)
	}
	// and create a donation stats master record
	// TODO
	cid := ""
	if contactID != nil {
		cid = *contactID
	}

	log.Infof("TODO create donation stats master record for page id %d and contact id %s", pageID, cid)

	return nil
}
