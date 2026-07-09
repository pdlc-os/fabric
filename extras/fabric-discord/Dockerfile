FROM golang:1.26 AS builder
WORKDIR /src

# Copy root module (needed by replace directive in extras/fabric-discord/go.mod)
COPY go.mod go.sum ./
COPY pkg/ pkg/
COPY proto/ proto/
COPY cmd/ cmd/
COPY internal/ internal/
COPY resources/ resources/
COPY harnesses/ harnesses/
COPY web/*.go web/

# Copy discord module
COPY extras/fabric-discord/ extras/fabric-discord/

# Build from the discord module directory
WORKDIR /src/extras/fabric-discord
RUN CGO_ENABLED=0 go build -tags no_embed_web -o /discord-bot ./cmd/fabric-plugin-discord

FROM gcr.io/distroless/static-debian12
COPY --from=builder /discord-bot /discord-bot
ENTRYPOINT ["/discord-bot", "--standalone"]
