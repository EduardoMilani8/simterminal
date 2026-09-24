.PHONY: test vet build run compare demo

test:
	go test ./...

vet:
	go vet ./...

build:
	go build -o simterminal ./cmd/simterminal

run:
	go run ./cmd/simterminal run -c exemplos/atual.yaml

compare:
	go run ./cmd/simterminal compare exemplos/atual.yaml exemplos/maisbalanca.yaml \
		exemplos/maisbalancas.yaml exemplos/maisdoca.yaml exemplos/agendamento.yaml \
		exemplos/balancaunica.yaml

demo:
	go run ./cmd/simterminal demo
