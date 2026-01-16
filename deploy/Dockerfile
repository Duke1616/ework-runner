FROM golang:1.24.1-alpine as build
MAINTAINER luankz
WORKDIR /app
COPY . .
# ENV GOPROXY="https://goproxy.cn,direct"
# ENV GOOS=linux GOARCH=amd64 CGO_ENABLED=0
RUN go mod tidy
RUN go build -a -o dist/ework-runner -ldflags="-w -s" main.go
FROM python:3.12.4-slim as deploy
WORKDIR /app
ENV EGO_DEBUG=true
COPY --from=build /app/dist/ework-runner .
COPY ./third_party ./third_party
RUN pip install --upgrade pip -i https://pypi.tuna.tsinghua.edu.cn/simple --no-cache-dir \
    && pip install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple -r /app/third_party/requirements.txt \
    && apt-get update && apt-get install vim jq curl -y
EXPOSE 8000