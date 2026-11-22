FROM golang:1.26.3-alpine AS build
WORKDIR /src
COPY order-service ./order-service
COPY contracts ./contracts
WORKDIR /src/order-service
RUN GOWORK=off CGO_ENABLED=0 go build -trimpath -o /out/order-service ./cmd/order-service
FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=build /out/ /app/
USER app
EXPOSE 8085 9094
ENTRYPOINT ["/app/order-service"]
