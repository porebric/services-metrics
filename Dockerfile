FROM golang:1.21-alpine

WORKDIR /app

RUN apk add --no-cache git

COPY . .

RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux
RUN go build ./cmd/services-metrics/main.go

CMD [ "/app/main" ]