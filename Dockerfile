FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/modbus2prometheus .

FROM scratch

ARG CA_CERTS_PACKAGE=ca-certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /app/modbus2prometheus /modbus2prometheus

CMD ["/modbus2prometheus"]
