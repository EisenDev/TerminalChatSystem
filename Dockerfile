FROM golang:1.23 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/teamchat-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/termichat ./cmd/client
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /out/termichat.exe ./cmd/client
RUN mkdir -p /out/package \
 && cp /out/termichat /out/package/termichat \
 && cp packaging/linux-client/install.sh /out/package/install.sh \
 && cp packaging/linux-client/README.txt /out/package/README.txt \
 && chmod +x /out/package/termichat /out/package/install.sh \
 && tar -czf /out/termichat-linux-amd64.tar.gz -C /out/package .

FROM ubuntu:24.04
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/teamchat-server /usr/local/bin/teamchat-server
COPY migrations /app/migrations
COPY scripts/seed.sql /app/scripts/seed.sql
COPY public /app/public
COPY --from=build /out/termichat-linux-amd64.tar.gz /app/public/downloads/termichat-linux-amd64.tar.gz
COPY --from=build /out/termichat.exe /app/public/downloads/termichat-windows-amd64.exe
COPY packaging/windows-client/install.ps1 /app/public/install.ps1

EXPOSE 18080

ENTRYPOINT ["/usr/local/bin/teamchat-server"]
