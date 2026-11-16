FROM golang:1.26.3-alpine AS build
WORKDIR /src
COPY . .
RUN GOWORK=off CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api
FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=build /out/ /app/
USER app
EXPOSE 8080 9090
ENTRYPOINT ["/app/api"]
