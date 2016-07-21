package justgiving

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/time/rate"

	"github.com/jackc/pgx"

	"github.com/homemade/justin"
	justin_models "github.com/homemade/justin/models"
)

var JGRL *rate.Limiter
var JGRLCtx context.Context
var JGRLCanc context.CancelFunc

func init() {
	// we setup a rate limit for JG API calls of 2 per second
	// TODO calculate his based on batch size and heartbeat env vars
	JGRL = rate.NewLimiter(rate.Limit(2), 2)
	JGRLCtx, JGRLCanc = context.WithCancel(context.Background())
}

func Shutdown() {
	// cancel any pending rate limiter
	JGRLCanc()

}

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

	defaultPagePriority := 0
	defaultPagePriority, err = getDefaultPagePriority(conn)
	if err != nil {
		return fmt.Errorf("error fetching default page priority from justgiving database %v", err)
	}

	// we update results in batches so as not to overload the justgiving api
	batchSize, err := strconv.Atoi(os.Getenv("JUSTIN_RESULTS_BATCH"))
	if batchSize < 1 || err != nil {
		return errors.New("missing or invalid JUSTIN_RESULTS_BATCH env var, expected integer value >= 1")
	}
	// retrieve the batch - this searches for non cancelled pages not updated in the last hour
	// results are then ordered on priority followed by the last updated timestamp
	// (the COALESCE postgres function handles null values)
	// finally the results are limited based on the batch size
	batch, err := conn.Query(`SELECT page_id FROM justgiving.page_priority
 WHERE priority <> 0 AND (fundraising_result_timestamp IS NULL OR fundraising_result_timestamp < (CURRENT_TIMESTAMP - INTERVAL '1 hours'))
 ORDER BY priority, COALESCE(fundraising_result_timestamp, TIMESTAMP '1970-01-01 00:00') LIMIT $1;`, batchSize)
	if err != nil {
		return fmt.Errorf("error querying justgiving.page_priority %v", err)
	}
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
	batch.Close()

	// next, retrieve events to sync
	rows, err := conn.Query("SELECT event_id FROM justgiving.event WHERE priority <> 0 ORDER BY priority;")
	if err != nil {
		return fmt.Errorf("error querying justgiving.event %v", err)
	}
	var events []uint
	for rows.Next() {
		var eventID uint
		if err = rows.Scan(&eventID); err != nil {
			return fmt.Errorf("error reading from justgiving.event %v", err)
		}
		if eventID > 0 {
			events = append(events, eventID)
		}
	}
	rows.Close()

	// if we have some events to sync...
	if len(events) > 0 {
		// retrieve pages for the events
		for _, e := range events {

			// we rate limit this call to the justgiving api
			if err = JGRL.Wait(JGRLCtx); err != nil {
				// just return on error - probably a legitimate shutdown by Heroku
				// (we don't want to fill up the job queue with these errors)
				return nil
			}

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
						_, err = conn.Exec(sql, p.ShortName(), p.ID())
						if err != nil {
							return fmt.Errorf("error updating justgiving.page %v", err)
						}
					}
				}

				// check if the current page is in the batch
				if inBatch(nextBatch, p.ID()) {

					// get the current year, month, day
					now := time.Now()
					day := now.Day()
					month := now.Month()
					year := now.Year()

					// retrieve the latest results (we rate limit this call to the justgiving api)
					if err = JGRL.Wait(JGRLCtx); err != nil {
						// just return on error - probably a legitimate shutdown by Heroku
						// (we don't want to fill up the job queue with these errors)
						return nil
					}

					serviceable := (p.ShortName() != "") // TODO investigate handling pages wih no short names
					var fr justin_models.FundraisingResults
					if serviceable {
						fr, err = svc.FundraisingPageResults(p)
						if err != nil {
							// if there was an error try and bump the priority of the page
							sql = `UPDATE justgiving.page_priority SET priority=priority+1 WHERE page_id=$1`
							conn.Exec(sql, p.ID())
							return fmt.Errorf("error fetching justgiving results for page id %d with short name `%s` %v", p.ID(), p.ShortName(), err)
						}
					}

					// if the page is cancelled or unserviceable set the priority to 0
					if fr.PageCancelled || !serviceable {
						sql = `UPDATE justgiving.page_priority SET priority=0 WHERE page_id=$1`
						_, err = conn.Exec(sql, p.ID())
						if err != nil {
							return fmt.Errorf("error updating justgiving.page_priority for cancelled page %v", err)
						}
					} else { // update the results
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
							_, err = conn.Exec(sql, fr.Target, fr.TotalRaisedPercentageOfTarget, fr.TotalRaisedOffline, fr.TotalRaisedOnline, fr.TotalRaisedSMS, fr.TotalEstimatedGiftAid, p.ID(), year, month, day)
							if err != nil {
								return fmt.Errorf("error updating justgiving.fundraising_result %v", err)
							}
						}
					}

					// update result timestamp
					sql = `UPDATE justgiving.page_priority SET fundraising_result_timestamp=CURRENT_TIMESTAMP WHERE page_id=$1`
					_, err = conn.Exec(sql, p.ID())
					if err != nil {
						return fmt.Errorf("error updating fundraising_result_timestamp on justgiving.page_priority %v", err)
					}

					// reset any pages which had their priority bumped due to a previous error but have now succeeded
					sql = `UPDATE justgiving.page_priority SET priority=$1 WHERE page_id=$2 AND priority > $3`
					_, err = conn.Exec(sql, defaultPagePriority, p.ID(), defaultPagePriority)
					if err != nil {
						return fmt.Errorf("error updating fundraising_result_timestamp on justgiving.page_priority %v", err)
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

func getDefaultPagePriority(conn *pgx.Conn) (int, error) {
	sql := `SELECT adsrc AS default_value
 FROM pg_catalog.pg_attrdef pad, pg_catalog.pg_attribute pat, pg_catalog.pg_class pc
 WHERE pc.relname='page_priority'
 AND pc.oid=pat.attrelid AND pat.attname='priority'
 AND pat.attrelid=pad.adrelid AND pat.attnum=pad.adnum;`
	var result int
	err := conn.QueryRow(sql).Scan(&result)
	return result, err
}
