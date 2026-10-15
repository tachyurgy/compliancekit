# Runtime image for the public demo. The binary and the built React app are
# produced on the build machine (see deploy/deploy-demo.sh) so the small demo
# host never runs a Go or Node toolchain. For a from-source build use the
# multi-stage Dockerfile at the repository root instead.
FROM busybox:1.36 AS prep
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY compliancekit-api /app/compliancekit-api
COPY web /app/web
# Named volumes inherit this directory's ownership on first mount, so the
# nonroot user can write the SQLite file and blobs.
COPY --from=prep --chown=65532:65532 /out/data /data
ENV STATIC_DIR=/app/web \
    DATABASE_URL=/data/ck.db \
    STORAGE_LOCAL_DIR=/data/blobs \
    AUTO_MIGRATE=true \
    PORT=8080
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/compliancekit-api"]
