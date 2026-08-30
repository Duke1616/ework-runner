FROM golang:1.25.0-alpine AS build
WORKDIR /app

# 拷贝依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 拷贝源码并编译
COPY . .

ARG SERVICE_PATH=.
ARG SERVICE_NAME=etask
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o dist/${SERVICE_NAME} -ldflags="-w -s" ${SERVICE_PATH}

FROM python:3.12.4-slim AS deploy
WORKDIR /app
ENV EGO_DEBUG=true

# 1. 系统依赖层
RUN apt-get update \
    && apt-get install --no-install-recommends vim jq curl openssh-client sshpass -y \
    && rm -rf /var/lib/apt/lists/*

# 2. Python 依赖层
COPY ./third_party/requirements.txt ./third_party/requirements.txt
RUN pip install --upgrade pip -i https://pypi.tuna.tsinghua.edu.cn/simple --no-cache-dir \
    && pip install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple -r /app/third_party/requirements.txt

# 3. 静态三方脚本与资源
COPY ./third_party ./third_party

# 4. 业务编译二进制产物
COPY --from=build /app/dist/${SERVICE_NAME} .

EXPOSE 8765 9002 9004

ENTRYPOINT ["./etask"]
CMD ["server", "--mode", "all", "--config", "/app/config/prod.yaml"]
