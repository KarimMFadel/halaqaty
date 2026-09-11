# syntax=docker/dockerfile:1
# API runtime for F-004 chat media parsing (ADR-022).
FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/halaqaty-api ./cmd/api

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg qpdf \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --no-create-home --uid 10001 halaqaty
COPY --from=build /out/halaqaty-api /usr/local/bin/halaqaty-api
USER halaqaty
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/halaqaty-api"]
