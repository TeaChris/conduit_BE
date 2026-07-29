# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o /bin/server ./cmd/server

# Runtime stage
FROM gcr.io/distroless/static-debian12

COPY --from=builder /bin/server /server
COPY --from=builder /app/migrations /migrations

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/server"]
