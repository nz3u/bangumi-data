# 构建阶段
FROM golang:1.25-alpine AS build
WORKDIR /src

# 利用层缓存：先拷贝依赖清单
COPY go.mod go.sum ./
RUN go mod download

# 拷贝源码（common 子模块内嵌进二进制）
COPY embedded.go ./
COPY cmd ./cmd
COPY internal ./internal
COPY common ./common

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bangumi ./cmd/bangumi

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