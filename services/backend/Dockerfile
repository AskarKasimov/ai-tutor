# Build context: the repository root. The backend imports the generated LLM
# gateway client from the sibling module services/shared (go.mod replace
# ../shared), so /shared sits next to /src.
FROM golang:1.26-alpine AS dev
WORKDIR /src
RUN go install github.com/air-verse/air@v1.67.4
COPY services/shared /shared
COPY services/backend/go.mod services/backend/go.sum ./
RUN go mod download
COPY services/backend/.air.toml ./
CMD ["air", "-c", ".air.toml"]

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY services/shared /shared
COPY services/backend/go.mod services/backend/go.sum ./
RUN go mod download
COPY services/backend/cmd ./cmd
COPY services/backend/internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /api ./cmd/api

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 65532 nonroot \
    && adduser -S -D -H -u 65532 -G nonroot nonroot
COPY --from=build /api /api
USER nonroot:nonroot
EXPOSE 8002
ENTRYPOINT ["/api"]
