FROM golang:1.21.1-alpine as build
MAINTAINER luankz
WORKDIR /app
COPY . .
ENV GOPROXY="https://goproxy.cn,direct"
# ENV GOOS=linux GOARCH=amd64 CGO_ENABLED=0
RUN go mod tidy
RUN go build -a -o dist/ework-runner -ldflags="-w -s" main.go
FROM alpine:latest as deploy
WORKDIR /app
COPY --from=build /app/dist/ework-runner .
EXPOSE 8000