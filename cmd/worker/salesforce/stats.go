package salesforce

import (
	"time"

	"github.com/jackc/pgx"
)

type DonationStatsDetail struct {
	TransactionDate  time.Time
	RaisedOfflineInc float64
	RaisedOnlineInc  float64
	RaisedSMSInc     float64
	EstimatedGiftAid float64
}

func Stats(conn *pgx.Conn, pageID uint, year int, month int, day int) (DonationStatsDetail, error) {
	var results DonationStatsDetail
	// TODO
	// 	sql := `SELECT transaction_date__c,
	//  raised_offline_incremental__c,
	//  estimated_gift_aid__c,
	//  raised_sms_incremental__c,
	//  transaction_date__c,
	//  raised_online_incremental__c)
	//  FROM salesforce.donation_stats__c
	// WHERE fundraising_page_id__c = $1 and transaction_date__c IS NOT NULL
	// ORDER BY transaction_date__c`
	return results, nil

}
