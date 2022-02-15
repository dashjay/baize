FROM golang:1.17 AS build

WORKDIR /go/baize-server

COPY go.mod go.sum ./

ENV GO111MODULE on
ENV GOPROXY https://goproxy.cn

RUN go env -w GO111MODULE=on \
    && go mod download

COPY ./pkg /go/baize-server/pkg
COPY ./cmd /go/baize-server/cmd

RUN go build -o /opt/baize-server cmd/baize-server/main.go

FROM library/ubuntu:20.04

COPY --from=build /opt/baize-server /usr/local/bin/baize-server


ENTRYPOINT ["/usr/local/bin/baize-server"]