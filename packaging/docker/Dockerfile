# syntax=docker/dockerfile:1
#
# The xqsyncd container image (docs/sync-server.md, "Docker"). It packages the binaries the release
# already cross-compiled rather than compiling again: the context holds linux_amd64/xqsyncd and
# linux_arm64/xqsyncd, and buildx picks the one for each platform. Nothing runs on the target
# platform, so a multi-arch build needs no emulation.

# The data and backup directories have to exist in the image, owned by the user the server runs
# as: a new volume takes the owner of the directory it is mounted over, and over a missing path it
# belongs to root, where the server could write neither its keys nor an admin backup. distroless
# has no shell to create them, so a throwaway stage on the build machine does.
FROM --platform=$BUILDPLATFORM busybox:1.36.1 AS prep
RUN mkdir -m 0700 /data /backup

# Static, no shell, no package manager: the server is one CGO-free binary and needs nothing else.
FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.title="xqsyncd" \
      org.opencontainers.image.description="Self-hosted sync server for xQuakShell. Stores only ciphertext it cannot open." \
      org.opencontainers.image.source="https://github.com/teoritty/xQuakShell" \
      org.opencontainers.image.documentation="https://github.com/teoritty/xQuakShell/blob/main/docs/sync-server.md"

COPY --chmod=0755 ${TARGETOS}_${TARGETARCH}/xqsyncd /usr/local/bin/xqsyncd
COPY --from=prep --chown=65532:65532 /data /data
COPY --from=prep --chown=65532:65532 /backup /backup

USER 65532:65532
VOLUME /data
EXPOSE 8443

ENTRYPOINT ["/usr/local/bin/xqsyncd"]
# serve --init creates the keys and the certificate on the first start and changes nothing after.
# --host only names the server in the printed connect link; set it to the name users connect to.
CMD ["serve", "--dir", "/data", "--addr", ":8443", "--init", "--host", "localhost"]
