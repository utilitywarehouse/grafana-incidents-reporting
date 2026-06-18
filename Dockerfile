FROM golang:1.26-alpine AS build
WORKDIR /go/src/github.com/utilitywarehouse/grafana-incidents-reporting
COPY . /go/src/github.com/utilitywarehouse/grafana-incidents-reporting

RUN go test ./... && CGO_ENABLED=0 go build -ldflags='-s -w' -o /grafana-incidents-reporting .

FROM alpine:3.24
RUN apk add --no-cache git openssh-client ca-certificates

RUN adduser -D -u 10001 reporter
USER reporter

COPY --from=build /grafana-incidents-reporting /usr/local/bin/grafana-incidents-reporting
ENTRYPOINT ["grafana-incidents-reporting"]
