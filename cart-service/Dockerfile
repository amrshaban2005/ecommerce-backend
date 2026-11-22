FROM golang:1.26.3-alpine AS build
WORKDIR /src
COPY cart-service ./cart-service
COPY contracts ./contracts
WORKDIR /src/cart-service
RUN GOWORK=off CGO_ENABLED=0 go build -trimpath -o /out/cart-service ./cmd/cart-service
FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=build /out/ /app/
USER app
EXPOSE 8083 9092
ENTRYPOINT ["/app/cart-service"]
