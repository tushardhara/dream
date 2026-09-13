# Build via `make container-build`: pinned Go toolchain, static linux binaries.
# Runtime image intentionally has no shell, package manager or embedded secrets.
FROM scratch
ARG REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/tushardhara/dream" \
      org.opencontainers.image.revision=$REVISION
COPY bin/container/hws /hws
COPY bin/container/hws-api /hws-api
COPY bin/container/hws-worker /hws-worker
COPY bin/container/hws-admin /hws-admin
USER 65532:65532
ENTRYPOINT ["/hws-api"]
CMD ["--help"]
