FROM golang:1.23-alpine AS builder

ARG APP_USER=appuser
ARG APP_UID
ARG VERSION=n/a

RUN apk add --no-cache git mercurial

WORKDIR $GOPATH/github.com/andrewslotin/youcast

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s -extldflags '-static' -X 'main.Version=${VERSION}'" -a -o /tmp/youcast .

FROM jrottenberg/ffmpeg:5.0-alpine

ARG APP_USER=appuser
ARG APP_UID

RUN adduser -D -h "/nonexistent" -u "${APP_UID}" "${APP_USER}"
USER $APP_USER:$APP_USER

COPY --from=builder /etc/ssl/certs /etc/ssl/certs
COPY --from=builder /tmp/youcast /bin/youcast

ENTRYPOINT ["/bin/youcast"]
