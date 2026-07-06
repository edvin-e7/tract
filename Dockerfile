# syntax=docker/dockerfile:1
#
# Single-binary Tract image. Mirrors scripts/build.sh: build the frontend, stage
# it into the Go embed dir, then compile one CGO-free static binary. The SQLite
# driver is modernc.org/sqlite (pure Go), so CGO_ENABLED=0 links cleanly and the
# runtime layer can be distroless-static — no libc, no shell, tiny attack surface.

# ---- frontend build ----
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- go build (CGO-free, static) ----
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Stage the freshly built frontend into the embed dir (replaces the committed
# placeholder), exactly as scripts/build.sh does for local builds.
RUN rm -rf cmd/tract/dist && mkdir -p cmd/tract/dist
COPY --from=frontend /app/frontend/dist/ cmd/tract/dist/
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/tract ./cmd/tract

# ---- runtime ----
# distroless/static ships CA roots + tzdata — the extractor fetches arbitrary
# HTTPS article URLs, so the certs are required (scratch would break TLS).
FROM gcr.io/distroless/static-debian12:latest
WORKDIR /data
COPY --from=build /out/tract /tract
# SQLite lives on a mounted volume so saved articles survive redeploys.
ENV TRACT_DB=/data/tract.db
EXPOSE 8080
ENTRYPOINT ["/tract"]
