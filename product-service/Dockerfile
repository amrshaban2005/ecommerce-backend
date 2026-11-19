FROM golang:1.26.3-alpine AS build
WORKDIR /src
COPY . .
RUN GOWORK=off CGO_ENABLED=0 go build -trimpath -o /out/product-service ./cmd/product-service
FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=build /out/ /app/
USER app
EXPOSE 8082 9091
ENTRYPOINT ["/app/product-service"]
