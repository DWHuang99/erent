ARG GO_VERSION=1.26.2

FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG BUILD_VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${BUILD_VERSION}" -o /out/api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck \
    && mkdir -p /out/api-logs \
    && touch /out/api-logs/.keep

FROM gcr.io/distroless/static-debian12:nonroot AS api
COPY --from=build /out/api /api
COPY --from=build /out/healthcheck /healthcheck
COPY --from=build --chown=65532:65532 /out/api-logs /var/log/ai-gateway
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=6 CMD ["/healthcheck"]
ENTRYPOINT ["/api"]

FROM gcr.io/distroless/static-debian12:nonroot AS gateway
COPY --from=build /out/gateway /gateway
COPY --from=build /out/healthcheck /healthcheck
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=6 CMD ["/healthcheck"]
ENTRYPOINT ["/gateway"]

FROM migrate/migrate:v4.19.1 AS migrations
COPY migrations /migrations
