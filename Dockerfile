FROM golang:1.17 AS builder

ARG APP_USER=appuser
ARG APP_UID

RUN adduser \
        --disabled-password \
        --gecos "" \
        --home "/nonexistent" \
        --shell "/sbin/nologin" \
        --no-create-home \
        --uid "${APP_UID}" \
        "${APP_USER}"

WORKDIR $GOPATH/github.com/andrewslotin/youcast

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-w -s -extldflags "-static"' -a -o /tmp/youcast .

FROM scratch

ARG APP_USER=appuser

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

USER $APP_USER:$APP_USER

COPY --from=builder /tmp/youcast /bin/youcast

CMD ["/bin/youcast"]
