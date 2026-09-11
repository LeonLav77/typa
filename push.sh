#!/usr/bin/env bash
# Build and push the typing image to the home-lab registry.
#
#   ./push.sh              build, test, push :latest
#   ./push.sh 1.2.0        also tag and push :1.2.0
#   ./push.sh --no-test    skip the test suite (not recommended)
#
# compose.yml pulls :latest, so a plain ./push.sh is a complete deploy
# artifact; the version tag is just a rollback point.
set -euo pipefail

REGISTRY="registry.leon-home-lab.ddns.net"
IMAGE="${REGISTRY}/typing"
PLATFORM="${PLATFORM:-linux/amd64}"   # server arch, not necessarily this machine's

cd "$(dirname "$0")"

VERSION=""
RUN_TESTS=1
for arg in "$@"; do
  case "$arg" in
    --no-test) RUN_TESTS=0 ;;
    -h|--help) sed -n '2,9p' "$0" | sed 's/^# \?//'; exit 0 ;;
    -*) echo "unknown option: $arg" >&2; exit 2 ;;
    *)  VERSION="$arg" ;;
  esac
done

say() { printf '\n\033[1;34m==>\033[0m %s\n' "$1"; }
die() { printf '\n\033[1;31merror:\033[0m %s\n' "$1" >&2; exit 1; }

# --- preflight ------------------------------------------------------------
command -v docker >/dev/null || die "docker not found"
docker info >/dev/null 2>&1 || die "docker daemon not running"

if [[ -n "$VERSION" && ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  die "version must look like 1.2.0 (got '$VERSION')"
fi

# The registry is only reachable on the VPN. Fail here with a clear message
# rather than after a five-minute build.
say "checking registry ${REGISTRY}"
code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "https://${REGISTRY}/v2/" || echo 000)
case "$code" in
  200|401) : ;;                       # 401 = up, needs login (handled below)
  000) die "cannot reach ${REGISTRY} — is the VPN connected?" ;;
  *)   die "registry returned HTTP ${code}" ;;
esac

# --- tests ----------------------------------------------------------------
if [[ "$RUN_TESTS" == 1 ]]; then
  say "running tests"
  if command -v go >/dev/null; then
    go vet ./... || die "go vet failed — not pushing"
    go test ./... || die "tests failed — not pushing"
  else
    # Refuse rather than skip: silently shipping untested code is worse than
    # stopping. Use --no-test to override deliberately.
    die "go not found (the suite can also run in docker), or pass --no-test"
  fi
fi

# --- build ----------------------------------------------------------------
TAGS=(-t "${IMAGE}:latest")
if [[ -n "$VERSION" ]]; then TAGS+=(-t "${IMAGE}:${VERSION}"); fi

say "building ${IMAGE}:latest${VERSION:+ and :$VERSION} for ${PLATFORM}"
docker build --platform "$PLATFORM" "${TAGS[@]}" . || die "build failed"

# Smoke-test the image before it goes anywhere: a container that cannot answer
# /healthz is not worth pushing. A tmpfs /data keeps the throwaway database out
# of any real volume.
say "smoke-testing the image"
cid=$(docker run -d --rm --tmpfs /data:uid=10001 -e DB_PATH=/data/smoke.db -P "${IMAGE}:latest")
trap 'docker rm -f "$cid" >/dev/null 2>&1 || true' EXIT
port=$(docker port "$cid" 8080/tcp | head -1 | sed 's/.*://')
ok=0
for _ in $(seq 40); do
  if curl -sf "http://127.0.0.1:${port}/healthz" >/dev/null 2>&1; then ok=1; break; fi
  sleep 0.5
done
[[ "$ok" == 1 ]] || { docker logs "$cid" | tail -20; die "image failed /healthz"; }
echo "  health: $(curl -s "http://127.0.0.1:${port}/healthz")"

# The health check only proves the process is up. Deal a test too, so a broken
# template or a missing embedded asset is caught here rather than in the browser.
curl -sf "http://127.0.0.1:${port}/api/test" >/dev/null \
  || { docker logs "$cid" | tail -20; die "image failed to deal a test"; }
curl -sf "http://127.0.0.1:${port}/" >/dev/null \
  || { docker logs "$cid" | tail -20; die "image failed to render the index page"; }
echo "  index and /api/test both answer"

docker rm -f "$cid" >/dev/null; trap - EXIT

# --- push -----------------------------------------------------------------
say "pushing to ${REGISTRY}"
# Trust docker's exit code. Progress goes to stderr, so scraping stdout for
# output is both wrong and unnecessary.
push() {
  local ref="$1"
  if ! docker push "$ref"; then
    echo
    echo "  if this says 'unauthorized' or 'denied', run:" >&2
    echo "    docker login ${REGISTRY}" >&2
    die "push failed for ${ref}"
  fi
}
push "${IMAGE}:latest"
if [[ -n "$VERSION" ]]; then push "${IMAGE}:${VERSION}"; fi

# --- done -----------------------------------------------------------------
digest=$(docker inspect --format='{{index .RepoDigests 0}}' "${IMAGE}:latest" 2>/dev/null || echo "?")
cat <<DONE

pushed  ${IMAGE}:latest${VERSION:+
pushed  ${IMAGE}:${VERSION}}
digest  ${digest}

Deploy on the server:
  docker compose pull && docker compose up -d
DONE
