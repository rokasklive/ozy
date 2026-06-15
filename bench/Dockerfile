# bench-runner Docker image — runs the scenario benchmark harness.
# Build: docker build -f bench/Dockerfile -t ozy-bench-runner .
FROM golang:1.26.4-alpine AS builder

RUN apk add --no-cache git sqlite

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /usr/local/bin/ozy ./cmd/ozy
RUN CGO_ENABLED=0 go build -o /usr/local/bin/ozy-bench ./cmd/ozy-bench

# Runtime image — includes Node.js for OpenCode.
FROM node:22.14-alpine

RUN apk add --no-cache git sqlite ca-certificates tzdata ripgrep

COPY --from=builder /usr/local/bin/ozy /usr/local/bin/ozy
COPY --from=builder /usr/local/bin/ozy-bench /usr/local/bin/ozy-bench

RUN npm install -g opencode-ai@1.17.7

# Bench configs and scenarios are mounted from the host.
WORKDIR /bench

ENTRYPOINT ["ozy-bench", "run"]
