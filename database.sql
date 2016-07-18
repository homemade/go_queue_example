
CREATE SCHEMA justgiving;

CREATE TABLE justgiving.event(
   charity_id 				INT      	NOT NULL,
   event_id 					INT      	NOT NULL,
   priority           INT       NOT NULL DEFAULT 9,
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
