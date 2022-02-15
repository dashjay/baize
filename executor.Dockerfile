FROM golang:1.17 AS build

WORKDIR /go/baize-executor

COPY go.mod go.sum ./

ENV GO111MODULE on
ENV GOPROXY https://goproxy.cn

RUN go env -w GO111MODULE=on \
    && go mod download

COPY ./pkg /go/baize-executor/pkg
COPY ./cmd /go/baize-executor/cmd

RUN go build -o /opt/baize-executor cmd/baize-executor/main.go

FROM dashjay/ubuntu:executor-base

COPY --from=build /opt/baize-executor /usr/local/bin/baize-executor

ENTRYPOINT ["/usr/local/bin/baize-executor"]