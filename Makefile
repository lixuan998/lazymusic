.PHONY: build test vet
build:
	CGO_ENABLED=1 go build -buildvcs=false -trimpath -o bin/lazymusic ./cmd/lazymusic
test:
	CGO_ENABLED=1 go test -count=1 -timeout=120s ./...
vet:
	CGO_ENABLED=1 go vet ./...
