ARG NODE_VERSION=26.7.0
ARG GO_VERSION=1.26.6
ARG ALPINE_VERSION=3.24.2

# Vite SPA: web/ -> web/dist.
FROM node:${NODE_VERSION}-alpine AS web
WORKDIR /web

COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit

# Build stamp for the frontend; Vite exposes only VITE_* variables.
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
ENV VITE_APP_VERSION=$VERSION \
    VITE_APP_COMMIT=$COMMIT \
    VITE_APP_BUILD_DATE=$BUILD_DATE

COPY web/ ./
RUN npm run build

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# go.su[m] is a glob: a module without dependencies has no go.sum.
COPY go.mod go.su[m] ./
RUN go mod download

COPY . .
# The built SPA is embedded through //go:embed all:dist.
COPY --from=web /web/dist ./web/dist

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
ARG GO_PACKAGE=""
# Main package: GO_PACKAGE, else the only directory in cmd/, else the root.
RUN PKG="${GO_PACKAGE}"; \
    if [ -z "$PKG" ]; then PKG=.; if [ -d cmd ]; then PKG="./cmd/$(ls cmd)"; fi; fi; \
    CGO_ENABLED=0 go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
      -o /out/app "$PKG"

FROM alpine:${ALPINE_VERSION}

ARG IMAGE_PACKAGES="xz"
# uid 65532 matches distroless nonroot, so host volume ownership stays valid.
RUN apk add --no-cache ca-certificates tzdata ${IMAGE_PACKAGES} && \
    addgroup -S -g 65532 nonroot && \
    adduser -S -u 65532 -G nonroot nonroot

COPY --from=build /out/app /usr/local/bin/app

ARG IMAGE_PORT=8080
EXPOSE ${IMAGE_PORT}
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/app"]
