package justgiving

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx"

	"github.com/homemade/justin"
)

func HeartBeat() error {

	// create justin service
	key := os.Getenv("JUSTIN_APIKEY")
	if key == "" {
		return errors.New("missing justin api key")
	}
	ctx := justin.APIKeyContext{
		APIKey:  key,
		Env:     justin.Live,
		Timeout: (time.Second * 20),
	}
	svc, err := justin.CreateWithAPIKey(ctx)
	if err != nil {
		return fmt.Errorf("error creating justin service %v", err)
	}

	// connect to justgiving database
	dbURL := os.Getenv("DATABASE_URL")
	connCfg, err := pgx.ParseURI(dbURL)
	if err != nil {
		return fmt.Errorf("error configuring connection to justgiving database %v", err)
	}
	conn, err := pgx.Connect(connCfg)
	if err != nil {
		return fmt.Errorf("error connecting to justgiving database %v", err)
	}
	defer conn.Close()

	// we update results in batches so as not to overload the justgiving api
	batchSize, err := strconv.Atoi(os.Getenv("JUSTIN_RESULTS_BATCH"))
	if batchSize < 1 || err != nil {
		return errors.New("missing or invalid JUSTIN_RESULTS_BATCH env var, expected integer value >= 1")
	}
	// retrieve the batch
	batch, err := conn.Query("SELECT page_id FROM justgiving.page_priority ORDER BY priority, fundraising_result_timestamp DESC LIMIT $1;", batchSize)
	if err != nil {
		return fmt.Errorf("error querying justgiving.page_priority %v", err)
	}
	defer batch.Close()
	var nextBatch []uint
	for batch.Next() {
		var pageID uint
		if err = batch.Scan(&pageID); err != nil {
			return fmt.Errorf("error reading from justgiving.page_priority %v", err)
		}
		if pageID > 0 {
			nextBatch = append(nextBatch, pageID)
		}
	}

	// next, retrieve events to sync
	rows, err := conn.Query("SELECT event_id FROM justgiving.event WHERE priority <> 0 ORDER BY priority;")
	if err != nil {
		return fmt.Errorf("error querying justgiving.event %v", err)
	}
	defer rows.Close()
	var events []uint
	for rows.Next() {
		var eventID uint
		if err := rows.Scan(&eventID); err != nil {
			return fmt.Errorf("error reading from justgiving.event %v", err)
		}
		if eventID > 0 {
			events = append(events, eventID)
		}
	}

	// if we have some events to sync...
	if len(events) > 0 {
		// retieve pages for the events
		for _, e := range events {
			next, err := svc.FundraisingPagesForEvent(e)
			if err != nil {
				return fmt.Errorf("error fetching pages for event id %d %v", e, err)
			}
			for _, p := range next {
				// check if we have already created records for this page
				sql := `SELECT page_short_name FROM justgiving.page WHERE page_id=$1`
				var shortName string
				err = conn.QueryRow(sql, p.ID()).Scan(&shortName)
				if err != nil {
					if err == pgx.ErrNoRows {
						// if not create them
						sql = `INSERT INTO justgiving.page (charity_id, event_id, page_id, page_short_name) VALUES($1,$2,$3,$4);`
						_, err = conn.Exec(sql, p.CharityID(), p.EventID(), p.ID(), p.ShortName())
						if err != nil {
							return fmt.Errorf("error creating justgiving.page %v", err)
						}
						sql = `INSERT INTO justgiving.page_priority (page_id) VALUES($1);`
						_, err = conn.Exec(sql, p.ID())
						if err != nil {
							return fmt.Errorf("error creating justgiving.page_priority %v", err)
						}
					} else {
						return fmt.Errorf("error querying justgiving.page %v", err)
					}
				} else {
					// if we have already stored the page, check if the short name has changed...
					if shortName != p.ShortName() { // ...and if it has, update it
						sql = `UPDATE justgiving.page SET page_short_name=$1,updated_timestamp=CURRENT_TIMESTAMP WHERE page_id=$2`
						_, err := conn.Exec(sql, p.ShortName(), p.ID())
						if err != nil {
							return fmt.Errorf("error updating justgiving.page %v", err)
						}
					}
				}

				// check if the current page is in the batch
				if inBatch(nextBatch, p.ID()) { // if so, update the results

					// get the current year, month, day
					now := time.Now()
					day := now.Day()
					month := now.Month()
					year := now.Year()

					// retrieve the latest results
					fr, err := svc.FundraisingPageResults(p)
					if err != nil {
						return fmt.Errorf("error fetching justgiving results %v", err)
					}

					// check if we have already created an initial results record for this page
					var res uint
					sql = `SELECT 1 FROM justgiving.fundraising_result WHERE page_id=$1 AND year = 0 and month = 0 and day = 0`
					err = conn.QueryRow(sql, p.ID()).Scan(&res)
					if err != nil {
						if err == pgx.ErrNoRows { // if not create one
							sql = `INSERT INTO justgiving.fundraising_result (page_id,year,month,day,target,total_raised_percentage_of_target,total_raised_offline,total_raised_online,total_raised_sms,total_estimated_gift_aid)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);`
							_, err = conn.Exec(sql, p.ID(), 0, 0, 0, fr.Target, fr.TotalRaisedPercentageOfTarget, fr.TotalRaisedOffline, fr.TotalRaisedOnline, fr.TotalRaisedSMS, fr.TotalEstimatedGiftAid)
							if err != nil {
								return fmt.Errorf("error creating initial justgiving.fundraising_result %v", err)
							}
						} else {
							return fmt.Errorf("error querying initial justgiving.fundraising_result %v", err)
						}
					}

					// check if we have already created a results record for this year/month/day
					sql = `SELECT 1 FROM justgiving.fundraising_result WHERE page_id=$1 AND year = $2 and month = $3 and day = $4`
					err = conn.QueryRow(sql, p.ID(), year, month, day).Scan(&res)
					if err != nil {
						if err == pgx.ErrNoRows { // if not create one
							sql = `INSERT INTO justgiving.fundraising_result (page_id,year,month,day,target,total_raised_percentage_of_target,total_raised_offline,total_raised_online,total_raised_sms,total_estimated_gift_aid)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);`
							_, err = conn.Exec(sql, p.ID(), year, month, day, fr.Target, fr.TotalRaisedPercentageOfTarget, fr.TotalRaisedOffline, fr.TotalRaisedOnline, fr.TotalRaisedSMS, fr.TotalEstimatedGiftAid)
							if err != nil {
								return fmt.Errorf("error creating justgiving.fundraising_result %v", err)
							}
						} else {
							return fmt.Errorf("error querying justgiving.fundraising_result %v", err)
						}
					} else { // otherwise update the existing record
						sql = `UPDATE justgiving.fundraising_result
 SET target=$1,total_raised_percentage_of_target=$2,total_raised_offline=$3,total_raised_online=$4,total_raised_sms=$5,total_estimated_gift_aid=$6,updated_timestamp=CURRENT_TIMESTAMP
 WHERE page_id=$7 AND year=$8 AND month=$9 AND day=$10`
						_, err := conn.Exec(sql, fr.Target, fr.TotalRaisedPercentageOfTarget, fr.TotalRaisedOffline, fr.TotalRaisedOnline, fr.TotalRaisedSMS, fr.TotalEstimatedGiftAid, p.ID(), year, month, day)
						if err != nil {
							return fmt.Errorf("error updating justgiving.fundraising_result %v", err)
						}
					}

				}

			}
		}
	}

	return nil

}

func inBatch(batch []uint, search uint) bool {
	for _, p := range batch {
		if search == p {
			return true
		}
	}
	return false
}
