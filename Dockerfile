FROM golang:1.17 AS builder

ARG APP_USER=appuser
ARG APP_UID

WORKDIR $GOPATH/github.com/andrewslotin/youcast

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-w -s -extldflags "-static"' -a -o /tmp/youcast .

FROM busybox

ARG APP_USER=appuser
ARG APP_UID

RUN adduser -D -h "/nonexistent" -u "${APP_UID}" "${APP_USER}"
USER $APP_USER:$APP_USER

COPY --from=builder /tmp/youcast /bin/youcast

CMD ["/bin/youcast"]
