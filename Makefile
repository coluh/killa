.PHONY: install dev-server dev-client build run test fmt clean

install:
	cd client && npm install
	cd server && go mod tidy

dev-server:
	cd server && go run ./cmd/killa -addr :8080

dev-client:
	cd client && npm run dev

build:
	cd client && npm run build
	cd server && go build -trimpath -o killa ./cmd/killa

run: build
	./server/killa -addr :8080 -static ./client/dist

test:
	cd server && go test ./...

loadtest:
	cd server && go run ./cmd/loadtest -url ws://127.0.0.1:8080/ws -clients 8 -seconds 10

fmt:
	cd server && gofmt -w .
	cd server && go vet ./...

clean:
	rm -rf client/dist server/killa
