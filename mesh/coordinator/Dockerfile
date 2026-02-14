FROM golang:1.21-alpine AS build
WORKDIR /app
COPY mesh/coordinator/ ./
RUN go build -o /coordinator .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /coordinator /usr/bin/coordinator
EXPOSE 9443
ENTRYPOINT ["/usr/bin/coordinator"]
ENV COORDINATOR_ADDR=:9443
ENV COORDINATOR_STORAGE=/data/coordinator.json
