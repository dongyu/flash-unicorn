# syntax=docker/dockerfile:1

ARG GO_VERSION=1.27
ARG DEBIAN_VERSION=bookworm-slim

FROM golang:${GO_VERSION}-bookworm AS builder

ARG MIGRATE_VERSION=v4.20.1
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w -X unicorn/internal/platform/buildinfo.Version=${VERSION} -X unicorn/internal/platform/buildinfo.Commit=${COMMIT} -X unicorn/internal/platform/buildinfo.Date=${BUILD_DATE}" \
      -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go install -tags "pgx5 sqlite" github.com/golang-migrate/migrate/v4/cmd/migrate@${MIGRATE_VERSION} && \
    cp "$(go env GOPATH)/bin/migrate" /out/migrate

FROM debian:${DEBIAN_VERSION} AS runtime

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata curl && \
    rm -rf /var/lib/apt/lists/* && \
    groupadd --system app && \
    useradd --system --gid app --home-dir /app --shell /usr/sbin/nologin app && \
    mkdir -p /app /data && chown -R app:app /app /data

WORKDIR /app

COPY --from=builder /out/server /usr/local/bin/server
COPY --from=builder /out/migrate /usr/local/bin/migrate
COPY --chown=app:app db ./db
COPY --chown=app:app scripts/migrate.sh ./scripts/migrate.sh
RUN chmod +x ./scripts/migrate.sh

USER app

ENV SERVER_BIN=/usr/local/bin/server \
    HTTP_ADDR=:3331 \
    SERVER_MODE=all

EXPOSE 3331

ENTRYPOINT ["/usr/local/bin/server"]
