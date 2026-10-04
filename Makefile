.PHONY: check fmt test vet build

check: fmt test vet

fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)

test:
	go test ./...

vet:
	go vet ./...

build:
	go build -o bin/nami ./cmd/nami
