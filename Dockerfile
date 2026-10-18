FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /build
COPY monero-pay/ .
RUN go mod download
RUN CGO_ENABLED=1 go build -o /monero-pay .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /monero-pay /usr/local/bin/monero-pay

RUN mkdir -p /data
WORKDIR /data

EXPOSE 8080

ENTRYPOINT ["monero-pay"]
