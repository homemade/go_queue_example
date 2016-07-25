
CREATE SCHEMA justgiving;

CREATE TABLE justgiving.event(
   charity_id 				INT      	NOT NULL,
   event_id 					INT      	NOT NULL,
   priority           INT       NOT NULL DEFAULT 9,
   name               VARCHAR(255),
   event_type         VARCHAR(255),
   location           VARCHAR(255),
   completion_date 	  TIMESTAMP,
   expiry_date  	    TIMESTAMP,
   start_date 	      TIMESTAMP,
	 created_timestamp 	TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	 updated_timestamp 	TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	 PRIMARY KEY (charity_id,event_id)
);
CREATE INDEX priority_event_index ON justgiving.event(priority);

CREATE TABLE justgiving.page(
  charity_id 				  INT      	   NOT NULL,
  event_id 					  INT      		 NOT NULL,
	page_id 						INT          NOT NULL,
	page_short_name 		VARCHAR(255) NOT NULL,
	created_timestamp 	TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_timestamp 	TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (charity_id,event_id,page_id)
);
CREATE INDEX page_short_name_page_index ON justgiving.page(page_short_name);

CREATE TABLE justgiving.page_priority(
	page_id 						          INT          NOT NULL,
  priority                      INT          NOT NULL DEFAULT 9,
	created_timestamp 	          TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_timestamp 	          TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
	fundraising_result_timestamp 	TIMESTAMP,
	PRIMARY KEY (page_id)
);
CREATE INDEX priority_page_priority_index ON justgiving.page_priority(priority);


CREATE TABLE justgiving.fundraising_result(
	page_id 													INT NOT NULL,
	year 															INT NOT NULL,
	month 														INT NOT NULL,
	day 															INT NOT NULL,
	target 														VARCHAR(48) NOT NULL,
	total_raised_percentage_of_target VARCHAR(48) NOT NULL,
	total_raised_offline           		VARCHAR(48) NOT NULL,
	total_raised_online            		VARCHAR(48) NOT NULL,
	total_raised_sms               		VARCHAR(48) NOT NULL,
	total_estimated_gift_aid       		VARCHAR(48) NOT NULL,
	created_timestamp 								TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_timestamp 								TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (page_id, year, month, day)
);

CREATE VIEW justgiving.event_page_fundraising_result AS
SELECT p.charity_id, p.event_id, e.name AS event_name, p.page_id, p.page_short_name, r.year, r.month, r.day, r.updated_timestamp,
CASE WHEN r.total_raised_offline IS NULL OR r.total_raised_offline='' THEN 0.0
	ELSE cast(r.total_raised_offline AS DOUBLE precision)
END AS raised_offline,
CASE WHEN r.total_raised_online IS NULL OR r.total_raised_online='' THEN 0.0
	ELSE cast(r.total_raised_online AS DOUBLE precision)
END AS raised_online,
CASE WHEN r.total_raised_sms IS NULL OR r.total_raised_sms='' THEN 0.0
	ELSE cast(r.total_raised_sms AS DOUBLE precision)
END AS raised_sms,
CASE WHEN r.total_estimated_gift_aid IS NULL OR r.total_estimated_gift_aid='' THEN 0.0
	ELSE cast(r.total_estimated_gift_aid AS DOUBLE precision)
END AS estimated_gift_aid,
CASE WHEN r.target IS NULL OR r.target='' THEN 0.0
	ELSE cast(r.target AS DOUBLE precision)
END AS target_amount
 FROM justgiving.fundraising_result r, justgiving.page p, justgiving.event e
WHERE p.page_id = r.page_id AND p.event_id = e.event_id
ORDER BY r.year DESC, r.month DESC, r.day DESC;
