FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod tidy
RUN go build -o homelink cmd/homelink-service/main.go

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/homelink .

# Expose HomeLink protocol port and API port
EXPOSE 8080 8081

CMD ["./homelink"]