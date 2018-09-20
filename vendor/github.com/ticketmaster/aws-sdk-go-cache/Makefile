all: get-deps unit

SDK_ONLY_PKGS=$(shell go list ./... | grep -v "/vendor/")

get-deps: get-deps-tests get-deps-verify
	@echo "go get SDK dependencies"
	@go get -v $(SDK_ONLY_PKGS)

get-deps-tests:
	@echo "go get SDK testing dependencies"
	go get -v github.com/alecthomas/gometalinter
	gometalinter --install
	go get -v github.com/smartystreets/goconvey

get-deps-verify:
	@echo "go get SDK verification utilities"
	@if [ \( -z "${SDK_GO_1_4}" \) -a \( -z "${SDK_GO_1_5}" \) ]; then  go get -v github.com/golang/lint/golint; else echo "skipped getting golint"; fi

lint:
	@echo "go lint SDK and vendor packages"
	@gometalinter --disable-all --enable=gofmt --enable=golint --enable=vet --enable=gosimple --enable=unconvert --deadline=4m ${SDK_ONLY_PKGS}
	@$(PRINT_OK)
