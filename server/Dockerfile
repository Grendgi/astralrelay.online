FROM golang:1.23-alpine AS builder
WORKDIR /app

COPY server/go.mod server/go.sum* ./
RUN go mod download

COPY server/ .
RUN go mod tidy && CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /server /server
EXPOSE 8080
CMD ["/server"]
