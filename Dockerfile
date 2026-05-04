FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Must not use "-o service": repo already has a package directory named service/.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/pramool-auction-service .

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /out/pramool-auction-service ./pramool-auction-service
EXPOSE 3103
CMD ["./pramool-auction-service"]
