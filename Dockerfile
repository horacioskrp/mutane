# ─────────────────────────────────────────────────────────────────────────────
#  Stage 1 — Build Go binary
# ─────────────────────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Dépendances système minimales (cgo désactivé → pas besoin de gcc)
RUN apk add --no-cache git

# Télécharger les modules en premier (layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Copier le reste du code source
COPY . .

# Build statique sans CGO
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /mutane ./cmd/server/

# ─────────────────────────────────────────────────────────────────────────────
#  Stage 2 — Image finale minimale
# ─────────────────────────────────────────────────────────────────────────────
FROM alpine:3.20

WORKDIR /app

# Certificats TLS (pour les appels HTTPS sortants si besoin)
RUN apk add --no-cache ca-certificates tzdata

# Copier le binaire compilé
COPY --from=builder /mutane .

# Copier les assets statiques et les migrations
COPY --from=builder /app/web/      ./web/
COPY --from=builder /app/migrations/ ./migrations/

# Répertoire uploads (monté en volume en production)
RUN mkdir -p /app/uploads

# Utilisateur non-root
RUN addgroup -S mutane && adduser -S mutane -G mutane
RUN chown -R mutane:mutane /app
USER mutane

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/ || exit 1

ENTRYPOINT ["/app/mutane"]
