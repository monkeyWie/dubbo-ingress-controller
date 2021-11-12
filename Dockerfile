FROM golang:1.17.3 AS builder
WORKDIR /src
COPY . .
ENV GOPROXY https://goproxy.cn
RUN go build -o main cmd/main.go

FROM busybox AS runner
ENV TZ=Asia/shanghai
WORKDIR /app
COPY --from=builder /src/main .
ENTRYPOINT  ["./main"]
