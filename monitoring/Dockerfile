FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY main.go moodle.go monitoring.go rabbitmq.go ./

RUN CGO_ENABLED=0 go build -o monitoring .

FROM alpine:3.22

WORKDIR /app

COPY --from=builder /app/monitoring .

CMD ["./monitoring"]
