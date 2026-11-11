# The Go API (the broker), plus a one-shot migrations image (target: migrate).
# Built and run by docker-compose.yml.
FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /api .

FROM golang:1.27-bookworm AS goose
RUN CGO_ENABLED=0 go install github.com/pressly/goose/v3/cmd/goose@v3.29.0

# Applies sql/schema to $DB_URL, then exits.
FROM alpine:3.20 AS migrate
COPY --from=goose /go/bin/goose /usr/local/bin/goose
COPY sql/schema /schema
ENTRYPOINT ["sh", "-c", "goose -dir /schema postgres \"$DB_URL\" up"]

# Runs as root: the engine creates the ring files as root in the shared /dev/shm.
FROM gcr.io/distroless/static-debian12 AS api
COPY --from=build /api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
