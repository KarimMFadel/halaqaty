# syntax=docker/dockerfile:1
# MinIO server built from the pinned official source tag (F-004 T014, ADR-021).
# The tag must stay pinned; never build from a moving branch.

FROM golang:1.25-alpine AS build

# Official MinIO source release tag. Keep the literal pin.
ARG MINIO_SOURCE_TAG=RELEASE.2025-10-15T17-29-55Z

RUN apk add --no-cache curl tar
WORKDIR /src
RUN curl -fsSL "https://github.com/minio/minio/archive/refs/tags/${MINIO_SOURCE_TAG}.tar.gz" -o source.tar.gz \
    && tar -xzf source.tar.gz --strip-components=1 \
    && rm source.tar.gz
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/minio .

FROM alpine:3.21
# curl stays in the runtime image for the compose healthcheck endpoint probe.
RUN apk add --no-cache ca-certificates curl \
    && addgroup -S minio && adduser -S minio -G minio \
    && mkdir -p /data && chown -R minio:minio /data
COPY --from=build /out/minio /usr/bin/minio
USER minio
EXPOSE 9000 9001
ENTRYPOINT ["/usr/bin/minio"]
CMD ["server", "/data", "--console-address", ":9001"]
