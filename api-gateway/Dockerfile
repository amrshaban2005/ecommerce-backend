FROM golang:1.26.3-alpine AS build
WORKDIR /src
COPY api-gateway ./api-gateway
COPY contracts ./contracts
WORKDIR /src/api-gateway
RUN GOWORK=off CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api-gateway
FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=build /out/ /app/
USER app
EXPOSE 8080
ENTRYPOINT ["/app/api"]
