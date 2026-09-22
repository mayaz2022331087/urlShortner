# ---------- Stage 1: Build ----------
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
# COPY go.sum ./   # uncomment if/when you add external dependencies

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# ---------- Stage 2: Run ----------
FROM alpine:3.20

WORKDIR /app

COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]