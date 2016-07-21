include Makefile.vars

.PHONY: vendor

vendor: $(GOPATH)/bin/godep
	@rm -rf Godeps
	@rm -rf vendor
	@go get -u github.com/bgentry/que-go
	@sudo chown vagrant:vagrant /mnt/GoWork/src/github.com/homemade
	@go get -u github.com/homemade/justin
	@go get -u github.com/jackc/pgx
	@go get -u github.com/Sirupsen/logrus
	@go get -u golang.org/x/sys/unix
	@go get -u github.com/go-kit/kit/log
	@go get -u github.com/kr/logfmt
	@go get -u golang.org/x/net/context
	@go get -u golang.org/x/time/rate
	@godep save ./...

$(GOPATH)/bin/godep:
	@go get github.com/tools/godep

run-heartbeat:
	@export DATABASE_URL=$(DATABASE_URL) && export HEARTBEAT=$(HEARTBEAT) && go run cmd/clock/main.go

run-workers:
	@export DATABASE_URL=$(DATABASE_URL) && export JUSTIN_APIKEY=$(JUSTIN_APIKEY) && export JUSTIN_CHARITY=$(JUSTIN_CHARITY) && export JUSTIN_RESULTS_BATCH=$(JUSTIN_RESULTS_BATCH) && go run cmd/worker/main.go

test-salesforce-worker:
	@export DATABASE_URL=$(DATABASE_URL) && export JUSTIN_APIKEY=$(JUSTIN_APIKEY) && export JUSTIN_CHARITY=$(JUSTIN_CHARITY) && go test -v --run TestSalesForce
