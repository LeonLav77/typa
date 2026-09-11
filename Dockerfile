# Two stages: build the static binary, then ship it on nothing.
#
# modernc.org/sqlite is a pure-Go SQLite, so CGO_ENABLED=0 gives a genuinely
# static binary and the runtime image needs no libc at all. Templates and
# static assets are //go:embed'ed into the binary, so there is nothing else to
# copy in.

FROM golang:1.25-alpine AS build
WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change, so an
# ordinary code edit does not re-download the module graph.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# -trimpath drops local filesystem paths from the binary; -w -s drop DWARF and
# the symbol table, which is a few MB of nothing useful in production.
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags='-w -s' \
      -o /out/typing .

# --- runtime --------------------------------------------------------------
FROM alpine:3.22

# curl for the container healthcheck; tzdata because runs are bucketed by
# local hour and weekday, so the container needs a real zoneinfo database.
RUN apk add --no-cache ca-certificates curl tzdata

# Unprivileged. The data directory is owned by it, since the SQLite file and
# its -wal/-shm siblings are created at runtime, not baked into the image.
RUN adduser -D -u 10001 typing \
 && mkdir -p /data \
 && chown typing:typing /data

COPY --from=build /out/typing /usr/local/bin/typing

USER typing
WORKDIR /data
VOLUME /data

ENV ADDR=:8080 \
    DB_PATH=/data/typing.db \
    TZ=Europe/Zagreb

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -sf http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["typing"]
