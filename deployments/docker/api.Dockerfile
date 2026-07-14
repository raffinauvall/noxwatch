FROM golang:1.26-alpine AS build
WORKDIR /app
COPY apps/api/go.mod apps/api/go.sum ./apps/api/
WORKDIR /app/apps/api
RUN go mod download
WORKDIR /app
COPY apps/api ./apps/api
COPY migrations ./migrations
WORKDIR /app/apps/api
RUN CGO_ENABLED=0 go build -o /app/api ./cmd/api

FROM alpine:3.23
WORKDIR /app
COPY --from=build /app/api /app/api
COPY migrations /app/migrations
EXPOSE 8080
CMD ["/app/api"]
