# syntax=docker/dockerfile:1.7

# ----------------------------------------------------------------------
# Stage 1: build the Vite SPA (web/ → web/dist). Copy package+lock first so
# npm ci caches when source churns but deps don't.
# ----------------------------------------------------------------------
FROM node:24.21.0 AS web
WORKDIR /web

COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit

# Build stamp shown in the footer. VITE_* prefix is mandatory (Vite only exposes
# those via import.meta.env). Defaults keep local `docker build` working.
ARG VERSION=dev
ARG COMMIT=local
ARG BUILD_DATE=
ENV VITE_APP_VERSION=$VERSION \
    VITE_APP_COMMIT=$COMMIT \
    VITE_APP_BUILD_DATE=$BUILD_DATE

COPY web/ ./
RUN npm run build

# ----------------------------------------------------------------------
# Stage 2: compile a static Go binary with the built SPA embedded.
# CGO_ENABLED=0 so the result runs on distroless/static (no libc).
# ----------------------------------------------------------------------
FROM golang:1.25.7 AS build

WORKDIR /src

# Cache module downloads in a separate layer — only re-runs when go.mod/sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Drop in the freshly built SPA. The committed web/dist/.gitkeep gets
# overwritten by Vite output here, then //go:embed all:dist picks it up.
COPY --from=web /web/dist ./web/dist

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/rutracker \
      ./cmd/rutracker

# ----------------------------------------------------------------------
# Stage 3: alpine runtime. Needs native `xz` on PATH — the parser stream-
# decompresses dumps via os/exec (pure-Go xz capped the pipeline at ~700 rows/s;
# native hits 30-60+ MB/s). ~10 MB image (vs distroless 2 MB). uid 65532 =
# distroless nonroot, so rt_api's `group_add: NAS_GID` reads the /dumps mount.
# ----------------------------------------------------------------------
FROM alpine:3.24.2

RUN apk add --no-cache xz ca-certificates tzdata && \
    addgroup -S -g 65532 nonroot && \
    adduser  -S -u 65532 -G nonroot nonroot

COPY --from=build /out/rutracker /rutracker

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/rutracker"]
CMD ["serve"]
