FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o homelink cmd/homelink-service/main.go

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/homelink .
CMD ["./homelink"]