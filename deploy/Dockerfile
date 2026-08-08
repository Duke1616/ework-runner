FROM golang:1.25.0-alpine AS build
WORKDIR /app
COPY . .
RUN go mod download

ARG SERVICE_PATH=.
ARG SERVICE_NAME=etask
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o dist/${SERVICE_NAME} -ldflags="-w -s" ${SERVICE_PATH}

FROM python:3.12.4-slim as deploy
WORKDIR /app
ENV EGO_DEBUG=true
ARG SERVICE_NAME=etask
COPY --from=build /app/dist/${SERVICE_NAME} .
COPY ./third_party ./third_party
RUN pip install --upgrade pip -i https://pypi.tuna.tsinghua.edu.cn/simple --no-cache-dir \
    && pip install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple -r /app/third_party/requirements.txt \
    && apt-get update \
    && apt-get install --no-install-recommends vim jq curl openssh-client sshpass -y \
    && rm -rf /var/lib/apt/lists/*
EXPOSE 8765 9002 9004

ENTRYPOINT ["./etask"]
CMD ["server", "--mode", "all", "--config", "/app/config/prod.yaml"]
