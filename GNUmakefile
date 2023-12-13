default: testacc

.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

.PHONY: build
build:
	go build -o bin/temporal-provider-temporal .

.PHONY: install
install:
	go install .

.PHONY: lint
lint:
	golangci-lint run --fix ./...

.PHONY: fmt
fmt:
	find . -name '*.go' -not -wholename './vendor/*' | while read -r file; do gofmt -w -s "$$file"; goimports -w "$$file"; done

.PHONY: doc
doc:
	go generate ./...
