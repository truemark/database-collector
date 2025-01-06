FROM golang:1.23-alpine as builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . ./
RUN go build cmd/collector/database-collector.go


FROM debian:stable-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/database-collector /app/database-collector
ENTRYPOINT ["/app/database-collector"]
