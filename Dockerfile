# Dockerfile
FROM golang:1.24 AS builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o rate-limiter ./cmd/server


FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/rate-limiter .
COPY config ./config
COPY internal/storage/*.lua ./internal/storage/
EXPOSE 8080
CMD ["./rate-limiter"]