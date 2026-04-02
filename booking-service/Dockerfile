FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN go build -o booking-service ./cmd

FROM alpine:3.20

WORKDIR /app
COPY --from=builder /app/booking-service /app/booking-service

EXPOSE 8082
CMD ["/app/booking-service"]
