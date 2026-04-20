FROM golang:1.23 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/teamchat-server ./cmd/server

FROM ubuntu:24.04
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/teamchat-server /usr/local/bin/teamchat-server
COPY migrations /app/migrations
COPY scripts/seed.sql /app/scripts/seed.sql

EXPOSE 18080

ENTRYPOINT ["/usr/local/bin/teamchat-server"]
