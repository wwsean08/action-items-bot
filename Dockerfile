FROM --platform=$BUILDPLATFORM cgr.dev/chainguard/go:latest AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/bot ./cmd/bot

FROM cgr.dev/chainguard/static:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/bot /usr/local/bin/bot
ENTRYPOINT ["/usr/local/bin/bot"]
