# Auction Service

Auction service built with Go, Gin, and MongoDB. New auctions start with `Active` status and are automatically closed with `Completed` status after the configured duration.

## Requirements

- Go 1.20 or newer for local execution
- Docker and Docker Compose for containerized execution

## Configuration

Configure `cmd/auction/.env`:

```env
AUCTION_DURATION=20s
MONGODB_URL=mongodb://admin:admin@mongodb:27017/auctions?authSource=admin
MONGODB_DB=auctions
```

`AUCTION_DURATION` accepts Go duration values such as `30s`, `5m`, or `1h`. If omitted or invalid, the service uses `5m`.

## Run With Docker Compose

```bash
docker compose up --build
```

The API is available at `http://localhost:8080` and MongoDB at `localhost:27017`.

Stop containers with:

```bash
docker compose down
```

## Run Locally

Start MongoDB, set `MONGODB_URL` to a reachable MongoDB instance, then run:

```bash
go mod tidy
```

Run tests with:

```bash
go test ./...
```

## Automatic Closing

The repository starts a goroutine after successful auction creation. The goroutine waits for `AUCTION_DURATION`, then updates the auction only when its current status is `Active`. This conditional update prevents an already closed auction from being overwritten.

On startup, the service loads auctions that are still `Active` and schedules the same closure. Auctions whose duration already elapsed while the process was down are closed immediately.
