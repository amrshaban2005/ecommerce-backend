FROM golang:1.26.3-alpine AS build
WORKDIR /src
COPY payment-service ./payment-service
COPY contracts ./contracts
WORKDIR /src/payment-service
RUN GOWORK=off CGO_ENABLED=0 go build -trimpath -o /out/payment-service ./cmd/payment-service
FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=build /out/ /app/
USER app
EXPOSE 8086 9095
ENTRYPOINT ["/app/payment-service"]
