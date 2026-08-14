# Multi-stage build for ollmo server (API + worker share this image).
# The same binary is reused by the worker service with a different entrypoint
# (`./ollmo worker`).

# ─── Build stage ───
FROM golang:1.26-alpine AS builder

# 国内网络默认使用 goproxy.cn 加速，可通过 build arg 覆盖
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/ollmo .

# ─── Runtime stage ───
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata curl \
    && addgroup -S app && adduser -S app -G app

ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /out/ollmo /app/ollmo
COPY --from=builder /app/config.yaml /app/config.yaml

RUN chown -R app:app /app
USER app

EXPOSE 8080
ENTRYPOINT ["/app/ollmo"]
CMD ["api"]
