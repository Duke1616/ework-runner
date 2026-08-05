FROM golang:1.25.0-alpine as build
MAINTAINER luankz
WORKDIR /app
COPY . .
# ENV GOPROXY="https://goproxy.cn,direct"
# ENV GOOS=linux GOARCH=amd64 CGO_ENABLED=0
RUN go mod download

ARG SERVICE_PATH=.
ARG SERVICE_NAME=etask
RUN go build -mod=readonly -a -o dist/${SERVICE_NAME} -ldflags="-w -s" ${SERVICE_PATH}

FROM python:3.12.4-slim as deploy
WORKDIR /app
ENV EGO_DEBUG=true
ARG SERVICE_NAME=etask
COPY --from=build /app/dist/${SERVICE_NAME} .
COPY ./third_party ./third_party
RUN pip install --upgrade pip -i https://pypi.tuna.tsinghua.edu.cn/simple --no-cache-dir \
    && pip install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple -r /app/third_party/requirements.txt \
    && apt-get update \
    && apt-get install --no-install-recommends vim jq curl -y \
    && rm -rf /var/lib/apt/lists/*
EXPOSE 8000
