FROM golang:1.17.3 AS builder
WORKDIR /src
COPY . .
ENV GOPROXY https://goproxy.cn
ENV CGO_ENABLED=0
RUN go build -ldflags "-w -s" -o main cmd/main.go

FROM busybox AS runner
ENV TZ=Asia/shanghai
WORKDIR /app
COPY --from=builder /src/main .
RUN chmod +x ./main
ENTRYPOINT ["./main"]
