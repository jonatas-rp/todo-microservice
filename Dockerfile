# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS build

WORKDIR /src

# Caches layers
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Build the application
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/todo-service ./cmd/api-server

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=build /out/todo-service /app/todo-service
COPY docs/openapi.json /app/docs/openapi.json

ENV PORT=8080
ENV DB_PATH=/tmp/todos.db
ENV LOG_LEVEL=INFO

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/todo-service"]
