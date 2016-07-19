package jgforce

import (
	"testing"

	"github.com/homemade/jgforce/cmd/worker/justgiving"
	"github.com/homemade/jgforce/cmd/worker/salesforce"
)

func TestJustGiving(t *testing.T) {
	err := justgiving.HeartBeat()
	if err != nil {
		t.Error(err)
	}
}

func TestSalesForce(t *testing.T) {
	err := salesforce.HeartBeat()
	if err != nil {
		t.Error(err)
	}
}
