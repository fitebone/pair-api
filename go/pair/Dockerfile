# syntax=docker/dockerfile:1
FROM golang:1.18-bullseye as builder

WORKDIR /app

#RUN apk update
#RUN apk add gcc

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Should test here
RUN go fmt $(go list ./... | grep -v /vendor/)
#RUN go vet $(go list ./... | grep -v /vendor/)
#RUN go test -race $(go list ./... | grep -v /vendor/)
RUN env GOOS=linux GOARCH=amd64 go build -o main .

###############################################################

FROM alpine:latest
#RUN apk --no-cache add ca-certificates

WORKDIR /app

# Add wait script to image
#ADD https://github.com/ufoscout/docker-compose-wait/releases/download/2.9.0/wait /wait
#RUN chmod +x /wait

COPY --from=builder /app/main ./
COPY --from=builder /app/.env ./
COPY --from=builder /app/cert/cert.pem ./cert/
COPY --from=builder /app/cert/pkey.pem ./cert/

EXPOSE 50051

CMD ["./main"]