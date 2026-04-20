APP=teamchat

.PHONY: server client test fmt migrate-up migrate-down seed bootstrap run-server-docker run-client-docker build-client-linux build-server-linux package-client-linux install-client

server:
	go run ./cmd/server

client:
	go run ./cmd/client

test:
	go test ./...

fmt:
	gofmt -w $(shell find . -name '*.go' -not -path './vendor/*')

migrate-up:
	migrate -path migrations -database "$$CHAT_DATABASE_URL" up

migrate-down:
	migrate -path migrations -database "$$CHAT_DATABASE_URL" down 1

seed:
	psql "$$CHAT_DATABASE_URL" -f scripts/seed.sql

bootstrap:
	sh scripts/bootstrap.sh

run-server-docker:
	sh scripts/run-server-docker.sh

run-client-docker:
	sh scripts/run-client-docker.sh

build-client-linux:
	sh scripts/build-client-linux.sh

build-server-linux:
	sh scripts/build-server-linux.sh

package-client-linux:
	sh scripts/package-client-linux.sh

install-client:
	sh scripts/install-client.sh
