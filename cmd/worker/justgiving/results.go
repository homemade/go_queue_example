package justgiving

import (
	"fmt"
	"time"

	"github.com/jackc/pgx"
)

type FundraisingResults struct {
	CharityID             uint
	EventID               uint
	EventName             string
	PageID                uint
	PageShortName         string
	Year                  int
	Month                 int
	Day                   int
	Timestamp             time.Time
	TotalRaisedOffline    float64
	TotalRaisedOnline     float64
	TotalRaisedSMS        float64
	TotalRaised           float64
	TotalEstimatedGiftAid float64
	Target                float64
}

func Results(conn *pgx.Conn, pageID uint, limitSQL string) ([]FundraisingResults, error) {
	var results []FundraisingResults
	sql := `SELECT * FROM justgiving.event_page_fundraising_result r WHERE page_id = $1
 ORDER BY r.year DESC, r.month DESC, r.day DESC`
	if limitSQL != "" {
		sql = sql + " " + limitSQL
	}
	rows, err := conn.Query(sql, pageID)
	if err != nil {
		return results, fmt.Errorf("error querying new justgiving.fundraising_result %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r FundraisingResults
		if err := rows.Scan(&r.CharityID, &r.EventID, &r.EventName, &r.PageID, &r.PageShortName,
			&r.Year, &r.Month, &r.Day, &r.Timestamp, &r.TotalRaisedOffline, &r.TotalRaisedOnline,
			&r.TotalRaisedSMS, &r.TotalEstimatedGiftAid, &r.Target); err != nil {
			return results, fmt.Errorf("error reading from justgiving.fundraising_result %v", err)
		}
		r.TotalRaised = r.TotalRaisedOffline + r.TotalRaisedOnline + r.TotalRaisedSMS
		results = append(results, r)
	}
	return results, nil

}
