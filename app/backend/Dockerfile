# build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o app

# runtime stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/app .
EXPOSE 8080
CMD ["./app"]
