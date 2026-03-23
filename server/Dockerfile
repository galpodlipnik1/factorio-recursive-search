FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/rbf-api ./main.go

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /out/rbf-api /usr/local/bin/rbf-api
USER appuser
EXPOSE 8080
ENTRYPOINT ["rbf-api"]