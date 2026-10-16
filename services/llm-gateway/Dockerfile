FROM golang:1.26-alpine AS build
WORKDIR /src/services
COPY services/shared/go.mod services/shared/go.sum ./shared/
COPY services/llm-gateway/go.mod services/llm-gateway/go.sum ./llm-gateway/
WORKDIR /src/services/llm-gateway
RUN GOWORK=off go mod download
WORKDIR /src/services
COPY services/shared ./shared
COPY services/llm-gateway ./llm-gateway
WORKDIR /src/services/llm-gateway
RUN GOWORK=off CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /llm-gateway ./cmd/llm-gateway
FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata && addgroup -S -g 65532 nonroot && adduser -S -D -H -u 65532 -G nonroot nonroot
COPY --from=build /llm-gateway /llm-gateway
USER nonroot:nonroot
EXPOSE 8010 8011
ENTRYPOINT ["/llm-gateway"]
