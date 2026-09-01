FROM --platform=$BUILDPLATFORM cgr.dev/chainguard/go:latest AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/bot \
    ./cmd/bot

FROM cgr.dev/chainguard/static:latest
COPY --from=builder /out/bot /usr/local/bin/bot
ENTRYPOINT ["/usr/local/bin/bot"]
