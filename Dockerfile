# 构建阶段（web/dist 由流水线 frontend job 预先构建、随构建上下文传入；
# 本地构建请先执行：cd web && npm install && npm run build）
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
WORKDIR /src

# 目标平台由 buildx 注入，Go 原生交叉编译，无需 QEMU 模拟
ARG TARGETOS TARGETARCH
ENV CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH

# 版本标识：发布流水线传入 git 标签（如 v0.2.0），本地构建默认 dev；
# 去掉前缀 v 后经 ldflags 写入 main.version
ARG VERSION=dev

# 利用层缓存：先拷贝依赖清单
COPY go.mod go.sum ./
RUN go mod download

# 拷贝源码（common 子模块与 web/dist 前端均内嵌进二进制）
COPY embedded.go ./
COPY cmd ./cmd
COPY internal ./internal
COPY common ./common
COPY web/embed.go ./web/
COPY web/dist ./web/dist

RUN VERSION="${VERSION#v}" && \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/bangumi ./cmd/bangumi

# 运行阶段
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/bangumi /app/bangumi

# 数据目录（SQLite 文件所在）
VOLUME ["/app/data"]
EXPOSE 8080

# 默认子命令为 serve，可通过 command 覆盖（如 import）
ENTRYPOINT ["/app/bangumi"]
CMD ["serve"]
