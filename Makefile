.PHONY: test run vet

test:
	go test ./...

vet:
	go vet ./...

run:
	go run ./cmd/simterminal
