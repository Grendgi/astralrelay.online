FROM golang:1.21-alpine AS build
WORKDIR /app
COPY mesh/backup-receiver/go.mod mesh/backup-receiver/main.go ./
RUN go build -o /receiver .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /receiver /usr/bin/receiver
EXPOSE 9100
ENTRYPOINT ["/usr/bin/receiver"]
ENV BACKUP_RECEIVER_ADDR=:9100
ENV BACKUP_DIR=/data/backups
