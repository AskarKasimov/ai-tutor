FROM golang:1.26-alpine AS dev
WORKDIR /work/voice-gateway
RUN GOMAXPROCS=2 go install -p 1 github.com/air-verse/air@v1.67.4
COPY services/shared /work/shared
COPY services/audio /work/audio
COPY services/voice-gateway/go.mod services/voice-gateway/go.sum ./
RUN GOWORK=off go mod download
COPY services/voice-gateway/.air.toml ./
RUN addgroup -S -g 65532 nonroot && adduser -S -D -H -u 65532 -G nonroot nonroot \
    && ln -s /work/voice-gateway/.air/voice-gateway /voice-gateway \
    && mkdir -p /work/voice-gateway/.air /var/lib/voice-gateway /go/cache \
    && chown -R nonroot:nonroot /go /work/voice-gateway/.air /var/lib/voice-gateway
ENV GOCACHE=/go/cache
USER nonroot:nonroot
CMD ["air", "-c", ".air.toml"]

FROM golang:1.26-alpine AS build
WORKDIR /src/services/voice-gateway
COPY services/shared /src/services/shared
COPY services/audio /src/services/audio
COPY services/voice-gateway/go.mod services/voice-gateway/go.sum ./
RUN GOWORK=off go mod download
COPY services/voice-gateway ./
RUN GOWORK=off CGO_ENABLED=0 GOMAXPROCS=2 go build -p 1 -trimpath -ldflags="-s -w" -o /voice-gateway ./cmd/voice-gateway

FROM alpine:3.24 AS runtime
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 65532 nonroot \
    && adduser -S -D -H -u 65532 -G nonroot nonroot \
    && mkdir -p /var/lib/voice-gateway \
    && chown nonroot:nonroot /var/lib/voice-gateway
COPY --from=build /voice-gateway /voice-gateway
USER nonroot:nonroot
EXPOSE 8020 8021
ENTRYPOINT ["/voice-gateway"]
