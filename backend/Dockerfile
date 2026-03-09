FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/server

FROM alpine:3.19

RUN addgroup -S mcd && adduser -S mcd -G mcd

WORKDIR /app

COPY --from=builder /app/server .

RUN mkdir -p /data/images /output/settlement && \
    chown -R mcd:mcd /app /data /output

USER mcd

EXPOSE 8080

CMD ["./server"]
