ARG ALPINE_VERSION=3.24.3

# The binary comes from go-build: bin/app, static, with the web UI embedded.
FROM alpine:${ALPINE_VERSION}

ARG IMAGE_PACKAGES="xz"
# uid 65532 matches distroless nonroot, so host volume ownership stays valid.
RUN apk add --no-cache ca-certificates tzdata ${IMAGE_PACKAGES} && \
    addgroup -S -g 65532 nonroot && \
    adduser -S -u 65532 -G nonroot nonroot

COPY bin/app /usr/local/bin/app

ARG IMAGE_PORT=8080
EXPOSE ${IMAGE_PORT}
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/app"]
