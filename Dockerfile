FROM golang:1.26-bookworm AS builder

WORKDIR /src

ENV GOPRIVATE=github.com/rudolfkova/*

COPY go.mod go.sum ./
COPY . .

# go.mod keeps a local replace for dev; production image fetches contracts from GitHub.
RUN go mod edit -dropreplace=github.com/rudolfkova/mytonprovider-backend/contracts && \
    go mod download && \
    go mod verify

ARG BUILD_TAGS=debug
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -tags="${BUILD_TAGS}" -ldflags="-s -w" -o /out/mtpo-backend ./cmd

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/mtpo-backend /usr/local/bin/mtpo-backend

EXPOSE 9092

ENTRYPOINT ["/usr/local/bin/mtpo-backend"]
