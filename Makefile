.PHONY: vendor

vendor: $(GOPATH)/bin/gvt
	@gvt update -all

$(GOPATH)/bin/gvt:
	@go get github.com/FiloSottile/gvt
