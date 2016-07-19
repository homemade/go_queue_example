package jgforce

import (
	"testing"

	"github.com/homemade/jgforce/cmd/worker/justgiving"
)

func TestJustGiving(t *testing.T) {
	err := justgiving.HeartBeat()
	if err != nil {
		t.Error(err)
	}
}
