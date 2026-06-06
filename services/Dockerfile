FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /app/bin/consumer-greptime  ./cmd/consumer-greptime
RUN go build -o /app/bin/consumer-doris     ./cmd/consumer-doris
RUN go build -o /app/bin/consumer-judge     ./cmd/consumer-judge
RUN go build -o /app/bin/consumer-alerting  ./cmd/consumer-alerting
RUN go build -o /app/bin/server             ./cmd/server
RUN go build -o /app/bin/eval               ./cmd/eval

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /app/bin/* /
