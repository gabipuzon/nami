.PHONY: check fmt test vet build prepare-analyzers

check: fmt test vet

fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)

test:
	go test ./...

vet:
	go vet ./...

prepare-analyzers:
	cd internal/analyzer/tsjsanalyzer/tooling && npm ci --ignore-scripts && node prepare.mjs

build:
	node internal/analyzer/tsjsanalyzer/tooling/prepare.mjs
	cd web && npm run build
	go build -tags webui -o bin/nami ./cmd/nami
