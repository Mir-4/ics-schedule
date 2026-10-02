FROM golang:1.23-alpine AS build

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY src ./src
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /uust-calendar ./src

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /uust-calendar /usr/local/bin/uust-calendar
COPY docker/run-calendar /usr/local/bin/run-calendar
COPY docker/entrypoint /usr/local/bin/entrypoint
COPY docker/crontab /etc/crontabs/root
RUN chmod 0755 /usr/local/bin/run-calendar /usr/local/bin/entrypoint

ENTRYPOINT ["/usr/local/bin/entrypoint"]
