FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
COPY .contracts-build /contracts
RUN go mod edit -replace mytonprovider-contracts=/contracts && \
    go mod download

COPY . .

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
