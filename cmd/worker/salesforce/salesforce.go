package salesforce

import (
	"errors"
	"fmt"
	"net/mail"
	"os"
	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/homemade/jgforce/cmd/worker/justgiving"
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
	sql := `SELECT c.sfid, c.jg_charity_id__c, c.event_id__c, c.fundraising_page_id__c,
 c.fundraising_page_url__c, c.fundraising_team_page_url__c,
 CASE WHEN c.fundraiser_jg_email__c IS NULL OR c.fundraiser_jg_email__c='' THEN c.email
 	ELSE c.fundraiser_jg_email__c
 END AS email
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
		if err = contacts.Scan(&r.ID, &r.CharityID, &r.EventID, &r.PageID, &r.PageURL, &r.TeamPageURL, &r.Email); err != nil {
			return fmt.Errorf("error reading from new salesforce.contacts %v", err)
		}
		// TODO handle team pages
		if r.ID != nil && *r.ID != "" && (r.TeamPageURL == nil || *r.TeamPageURL == "") {
			crecs = append(crecs, r)
		}
	}
	contacts.Close()

	// try and find a justgiving fundraising page for the new contacts

	for _, c := range crecs {
		// get salesforce contact id for reference
		sfcid := ""
		if c.ID != nil {
			sfcid = *c.ID
		}
		// our order of precedence for the search is:
		// 1. Try ids

		// try and use default charity id if none is provided
		rawCharityID := 0
		if c.CharityID == nil || *c.CharityID == "" {
			rawCharityID, err = strconv.Atoi(os.Getenv("JUSTIN_CHARITY"))
			if err != nil {
				log.Warnf("failed to set charity id from default %s %v", os.Getenv("JUSTIN_CHARITY"), err)
			}
		} else {
			rawCharityID, err = strconv.Atoi(*c.CharityID)
			if err != nil {
				log.Warnf("invalid charity id in salesforce contact %s %v", sfcid, err)
			}
		}
		charityID := uint(rawCharityID)

		rawEventID := 0
		if c.EventID != nil && *c.EventID != "" {
			rawEventID, err = strconv.Atoi(*c.EventID)
			if err != nil {
				log.Warnf("invalid event id in salesforce contact %s %v", sfcid, err)
			}
		}
		eventID := uint(rawEventID)

		rawPageID := 0
		if c.PageID != nil && *c.PageID != "" {
			rawPageID, err = strconv.Atoi(*c.PageID)
			if err != nil {
				log.Warnf("invalid page id in salesforce contact %s %v", sfcid, err)
			}
		}
		pageID := uint(rawPageID)

		var found bool
		found, err = searchForPageUsingID(svc, conn, charityID, eventID, pageID, c.ID)
		if err != nil {
			return err
		}
		if !found {
			// 2. If we couldn't find a match with the id, if we can, try the short name
			shortName := ""
			// TODO pending data import
			found, err = searchForPageUsingShortName(svc, conn, shortName)
			if err != nil {
				return err
			}
			if !found {
				// 3. Finally as a fallback try and use the email address of the contact
				if c.Email != nil {
					_, err = searchForPageUsingEmail(svc, conn, charityID, c)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	// NOTE: the search functions handle creation of donation stats master records when a matching page is found

	// update donation stats detail records (and check if the page name needs updating on the master record)
	// first get a list of the page ids and their last update timestamp
	rows, err := conn.Query("SELECT fundraising_page_id__c,MAX(transaction_date__c) FROM salesforce.donation_stats__c GROUP BY fundraising_page_id__c;")
	if err != nil {
		return fmt.Errorf("error querying pages from salesforce.donation_stats__c %v", err)
	}
	var pages []struct {
		id string
		ts *time.Time
	}
	for rows.Next() {
		var pageID *string
		var transDate *time.Time
		if err = rows.Scan(&pageID, &transDate); err != nil {
			return fmt.Errorf("error reading page id and transaction date from salesforce.donation_stats__c %v", err)
		}
		if pageID != nil && *pageID != "" {
			pages = append(pages,
				struct {
					id string
					ts *time.Time
				}{*pageID, transDate})
		}
	}
	rows.Close()

	// then fetch the results for each page
	for _, p := range pages {
		var results []justgiving.FundraisingResults
		var pid int
		pid, err = strconv.Atoi(p.id)
		if err != nil {
			return fmt.Errorf("error reading justgiving fundraising results for page %s %v", p.id, err)
		}
		results, err = justgiving.Results(conn, uint(pid), "")
		if len(results) > 0 {
			// check if the page name needs updating on the master record (all items in the results have the latest page name through the view that is used)
			if results[0].PageShortName != "" {
				psn := "https://www.justgiving.com/fundraising/" + results[0].PageShortName
				sql = `UPDATE salesforce.donation_stats__c SET fundraising_page_url__c = $2
			 WHERE fundraising_page_id__c = $1 AND transaction_date__c IS NULL
			 AND (fundraising_page_url__c IS NULL OR fundraising_page_url__c <> $2);`
				_, err = conn.Exec(sql, p.id, psn)
				if err != nil {
					return fmt.Errorf("error updating page short name for page id %s on initial salesforce.donation_stats__c record %v", p.id, err)
				}
			}
			// for the non initial results records (incremental records) -  query the salesforce results for the matching year, month, day
			if len(results) > 1 {
				// justgiving results are in descending order (we need to handle them in ascending order)
				// - we also skip the first initial/master record (index length-1)
				for i := len(results) - 2; i >= 0; i-- {
					// check if we need to sync this record
					fr := results[i]
					if p.ts == nil || fr.Timestamp.After(*p.ts) {
						// first retrieve the current salesforce amounts
						var contactID *string
						var currRaisedOnline, currRaisedSMS, currRaisedOffline, currEstimatedGiftAid, currTargetAmount *float64
						sql = `SELECT contact_id, raised_online, raised_sms, raised_offline, estimated_gift_aid, target_amount
		FROM salesforce.contact_page_fundraising_result WHERE page_id = $1;`
						err = conn.QueryRow(sql, &p.id).Scan(&contactID, &currRaisedOnline, &currRaisedSMS, &currRaisedOffline, &currEstimatedGiftAid, &currTargetAmount)
						if err != nil {
							return fmt.Errorf("error reading salesforce.contact_page_fundraising_result record for page id %s %v", p.id, err)
						}
						if contactID == nil {
							return fmt.Errorf("missing contact id when reading salesforce.contact_page_fundraising_result for page id %s", p.id)
						}
						if currRaisedOnline == nil {
							return fmt.Errorf("missing raised online amount reading salesforce.contact_page_fundraising_result record for page id %s", p.id)
						}
						if currRaisedSMS == nil {
							return fmt.Errorf("missing raised sms amount reading salesforce.contact_page_fundraising_result record for page id %s", p.id)
						}
						if currRaisedOffline == nil {
							return fmt.Errorf("missing raised offline amount reading salesforce.contact_page_fundraising_result record for page id %s", p.id)
						}
						if currEstimatedGiftAid == nil {
							return fmt.Errorf("missing estimated gift aid amount reading salesforce.contact_page_fundraising_result record for page id %s", p.id)
						}
						if currTargetAmount == nil {
							return fmt.Errorf("missing target amount reading salesforce.contact_page_fundraising_result record for page id %s", p.id)
						}
						// check if anything has changed
						diffRaisedOnline := fr.TotalRaisedOnline - *currRaisedOnline
						diffRaisedSMS := fr.TotalRaisedSMS - *currRaisedSMS
						diffRaisedOffline := fr.TotalRaisedOffline - *currRaisedOffline
						diffEstimatedGiftAid := fr.TotalEstimatedGiftAid - *currEstimatedGiftAid
						diffTargetAmount := fr.Target - *currTargetAmount

						// TODO neater floating point comparison
						if diffRaisedOnline > 0.01 || diffRaisedSMS > 0.01 || diffRaisedOffline > 0.01 || diffEstimatedGiftAid > -0.01 || diffTargetAmount > 0.01 ||
							diffRaisedOnline < -0.01 || diffRaisedSMS < -0.01 || diffRaisedOffline < -0.01 || diffEstimatedGiftAid < -0.01 || diffTargetAmount < -0.01 {
							log.Infof("inserting donation stats detail record for page id %s and year %d month %d and day %d", p.id, fr.Year, fr.Month, fr.Day)
							// insert the salesforce record
							sql = `INSERT INTO salesforce.donation_stats__c
	 (fundraising_page_id__c, related_contact_record__c, transaction_date__c, raised_online_incremental__c, raised_sms_incremental__c, raised_offline_incremental__c, estimated_gift_aid__c, pledge_amount_revised__c)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8);`
							_, err = conn.Exec(sql, p.id, *contactID, fr.Timestamp, diffRaisedOnline, diffRaisedSMS, diffRaisedOffline, diffEstimatedGiftAid, diffTargetAmount)
							if err != nil {
								return fmt.Errorf("error inserting incremental salesforce.donation_stats__c record for page id %s and year %d month %d and day %d %v", p.id, fr.Year, fr.Month, fr.Day, err)
							}
							log.Infof("rationale: %f %f %f | %f %f %f | %f %f %f | %f %f %f | %f %f %f | %v %v",
								diffRaisedOnline, fr.TotalRaisedOnline, *currRaisedOnline,
								diffRaisedSMS, fr.TotalRaisedSMS, *currRaisedSMS,
								diffRaisedOffline, fr.TotalRaisedOffline, *currRaisedOffline,
								diffEstimatedGiftAid, fr.TotalEstimatedGiftAid, *currEstimatedGiftAid,
								diffTargetAmount, fr.Target, *currTargetAmount,
								fr.Timestamp, p.ts)
						}
					}
				}
			}
		}
	}

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
		// if there is a match handle it...
		return true, handleMatch(conn, pageID, contactID)
	}
	if err == pgx.ErrNoRows {
		// if there is no match and we have a charity id and event id
		// check the event - we might want to add it
		// (hopefully we will then find a match later - once the events pages are retrieved)
		if charityID > 0 && eventID > 0 {
			err = checkEvent(svc, conn, charityID, eventID)
			if err != nil {
				return false, err
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

func searchForPageUsingEmail(svc *justin.Service, conn *pgx.Conn, charityID uint, c ContactRecord) (bool, error) {
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

	if charityID == 0 {
		log.Warn("missing charity id in call to searchForPageUsingEmail")
		return false, nil
	}

	fprs, err := svc.FundraisingPagesForCharityAndUser(charityID, *account)

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
				// check page is active (has some donations)
				var fres []justgiving.FundraisingResults
				fres, err = justgiving.Results(conn, p.ID(), "LIMIT 1")
				if err != nil {
					return false, err
				}
				if len(fres) == 1 && fres[0].TotalRaised > 0 {
					matchedIndex = i
					matchedCount = matchedCount + 1
				}
			}
		}
		if matchedCount == 1 {
			p := fprs[matchedIndex]
			return searchForPageUsingID(svc, conn, p.CharityID(), p.EventID(), p.ID(), c.ID)
		}

		// if there is no match then check the event - we might want to add it
		if matchedCount < 1 {
			for _, p := range fprs {
				checkEvent(svc, conn, p.CharityID(), p.EventID())
				if err != nil {
					return false, err
				}
			}
		}
	}

	return false, nil
}

func eventIDs(conn *pgx.Conn) ([]uint, error) {
	rows, err := conn.Query("SELECT event_id FROM justgiving.event WHERE priority > 0 ORDER BY priority;")
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

func checkEvent(svc *justin.Service, conn *pgx.Conn, charityID uint, eventID uint) error {
	// make sure this event doesn't already exist in our database
	eventIDs, err := eventIDs(conn)
	if err != nil {
		return err
	}
	if !in(eventIDs, eventID) {
		// retrieve event from justgiving api
		event, err := svc.Event(eventID)
		if err != nil {
			return fmt.Errorf("error fetching event %d from justgiving %v", eventID, err)
		}
		eventStartDate, err := event.ParseStartDate()
		if err != nil {
			return fmt.Errorf("error parsing event start date %s as returned from from justgiving %v", event.StartDate, err)
		}
		// check we have events like this
		sql := `SELECT event_id FROM justgiving.event WHERE priority > 0 AND start_date=$1 LIMIT 1`
		res := 0
		err = conn.QueryRow(sql, eventStartDate).Scan(&res)
		if err == nil {
			var eventCompletionDate time.Time
			eventCompletionDate, err = event.ParseCompletionDate()
			if err != nil {
				return fmt.Errorf("error parsing event completion date %s as returned from from justgiving %v", event.CompletionDate, err)
			}
			var eventExpiryDate time.Time
			eventExpiryDate, err = event.ParseExpiryDate()
			if err != nil {
				return fmt.Errorf("error parsing event expiry date %s as returned from from justgiving %v", event.ExpiryDate, err)
			}
			sql := `INSERT INTO justgiving.event (charity_id, event_id, name, event_type, location, completion_date, expiry_date, start_date) VALUES($1,$2,$3,$4,$5,$6,$7,$8);`
			_, err = conn.Exec(sql, charityID, eventID, event.Name, event.Type, event.Location, eventCompletionDate, eventExpiryDate, eventStartDate)
			if err != nil {
				return fmt.Errorf("error inserting justgiving.event %d %d %v", charityID, eventID, err)
			}
		} else {
			if err != pgx.ErrNoRows {
				return fmt.Errorf("error checking event %d against justgiving.event %v", eventID, err)
			}
		}
	}
	return nil
}

func handleMatch(conn *pgx.Conn, pageID uint, contactID *string) error {
	// bump the page priority so we refresh its results more often (except if the page is cancelled or unserviceable i.e. priority is 0)
	sql := `UPDATE justgiving.page_priority SET priority=5 WHERE page_id=$1 AND priority <> 0`
	_, err := conn.Exec(sql, pageID)
	if err != nil {
		return fmt.Errorf("error updating justgiving.page_priority to 5 for page id %d %v", pageID, err)
	}
	// check the page is active (has some donations)
	var fres []justgiving.FundraisingResults
	fres, err = justgiving.Results(conn, pageID, "")
	if err != nil {
		return err
	}
	// if it is active...
	if contactID != nil && len(fres) > 0 && fres[0].TotalRaised > 0 {
		// and we haven't already associated this page with someone...
		var rec *int
		sql = `SELECT 1 FROM salesforce.donation_stats__c WHERE fundraising_page_id__c = $1 AND transaction_date__c IS NULL;`
		err = conn.QueryRow(sql, strconv.FormatInt(int64(pageID), 10)).Scan(&rec)
		if err == pgx.ErrNoRows { // create a donation stats master record
			initRes := fres[len(fres)-1]
			log.Infof("inserting donation stats master record for page id %d", pageID)
			sql = `INSERT INTO salesforce.donation_stats__c
	 (fundraising_page_id__c, related_contact_record__c, initial_raised_online__c,
		initial_raised_sms__c, initial_raised_offline__c, intial_estimated_gift_aid__c, initial_pledge_amount__c,
		fundraising_portal_used__c, event_id__c, jg_charity_id__c, event_name__c)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);`
			_, err = conn.Exec(sql, strconv.FormatInt(int64(pageID), 10), *contactID, initRes.TotalRaisedOnline,
				initRes.TotalRaisedSMS, initRes.TotalRaisedOffline, initRes.TotalEstimatedGiftAid, initRes.Target,
				"Just Giving", strconv.FormatInt(int64(initRes.EventID), 10), strconv.FormatInt(int64(initRes.CharityID), 10), initRes.EventName)
			if err != nil {
				return fmt.Errorf("error creating initial salesforce.donation_stats__c %v", err)
			}
		} else {
			if err != nil {
				return fmt.Errorf("error checking for existing association with salesforce.donation_stats__c %v", err)
			}
		}
	}

	return nil
}
