FROM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/ ./cmd/api ./cmd/admin ./cmd/migrate

FROM alpine:3.23@sha256:469c9ea410b9215eaee7976ba6c0639039b444be8e5d638f7abbada189971eb2 AS runtime
WORKDIR /app
USER 65532:65532

FROM runtime AS tools
COPY --from=build /out/admin /out/migrate ./
COPY migrations ./migrations
ENTRYPOINT ["/app/migrate"]

FROM runtime AS api
# Go's default TLS verifier on Alpine uses the distribution CA bundle.
RUN test -s /etc/ssl/certs/ca-certificates.crt && test -r /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/api ./
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/readyz || exit 1
ENTRYPOINT ["/app/api"]
