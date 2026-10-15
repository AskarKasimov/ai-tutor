FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /llm-gateway ./cmd/llm-gateway

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 65532 nonroot \
    && adduser -S -D -H -u 65532 -G nonroot nonroot
COPY --from=build /llm-gateway /llm-gateway
USER nonroot:nonroot
EXPOSE 8010
ENTRYPOINT ["/llm-gateway"]
