# bench-runner Docker image — runs the scenario benchmark harness.
# Build: docker build -f bench/Dockerfile -t ozy-bench-runner .
FROM golang:1.26.4-alpine AS builder

RUN apk add --no-cache git sqlite

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /usr/local/bin/ozy ./cmd/ozy
RUN CGO_ENABLED=0 go build -o /usr/local/bin/ozy-bench ./cmd/ozy-bench

# Runtime image — Debian glibc (not alpine/musl) so FastEmbed + onnxruntime
# wheels install, plus Node.js for OpenCode. The musl base had no reliable onnx
# wheels, which forced ozy to lexical-only retrieval (old D9); this image bakes
# the semantic embedder at build time instead (see below).
FROM node:22-bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
      git sqlite3 ca-certificates tzdata ripgrep \
      python3 python3-venv python3-pip \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/bin/ozy /usr/local/bin/ozy
COPY --from=builder /usr/local/bin/ozy-bench /usr/local/bin/ozy-bench

# The pinned OpenCode version — also recorded in run provenance.
ENV OPENCODE_VERSION=1.17.7
RUN npm install -g opencode-ai@${OPENCODE_VERSION}

# Warm OpenCode's first-run state at build time (registry reachable) so any
# lazily-fetched provider SDK or model catalog is baked into the image rather
# than fetched at run time.
RUN opencode --version && (opencode models >/dev/null 2>&1 || true)

# Hermeticity gate: with the npm registry unroutable, OpenCode must still reach
# its ready-to-run state (resolve its model catalog). If reaching it needs a
# package fetch, the *build* fails here — never the run (scenario-bench: "Broken
# build-time state fails the build, not the run").
#
# At run time the harness itself pins npm_config_registry unroutable for the
# agent process (see internal/bench/runner.go): OpenCode's opportunistic plugin
# auto-install is then skipped, so the model gateway is the only runtime egress
# and no .npm/_cacache debris lands in run dirs.
RUN npm_config_registry=http://127.0.0.1:0 opencode models >/dev/null \
    || (echo "HERMETICITY FAILURE: opencode needs the npm registry to reach ready-to-run state" >&2 && exit 1)

# Bench configs and scenarios are mounted from the host. The ≥500-tool corpus is
# baked in so the image is self-contained even without the compose mount (the
# compose file also mounts ./corpus read-only, which shadows this with identical
# data). OZY_BENCH_CORPUS_DIR points the harness at it.
COPY bench/corpus /bench/corpus
ENV OZY_BENCH_CORPUS_DIR=/bench/corpus

# --- Semantic retrieval, baked at BUILD time (bench-enable-semantic, revisits D9) ---
# ozy ships hybrid semantic+lexical retrieval, semantic ON by default. The embedder
# is FastEmbed (Python venv) + the BAAI/bge-small-en-v1.5 model. We provision the
# venv and download the model HERE (network on) into a fixed state dir; at run time
# ozy reuses them offline and `ozy index` builds a real vector index. Runtime egress
# stays model-gateway-only.
#
# XDG_STATE_HOME fixes the venv location so build and run resolve the same one.
# OZY_SIDECAR_PYTHON pins the base interpreter so the provisioner never invokes uv
# (which could fetch a Python at run time — a hermeticity breach). OZY_SIDECAR_SOURCE
# points at the copied sidecar package that `python -m sidecar` runs.
COPY sidecar /opt/ozy/sidecar
ENV XDG_STATE_HOME=/bench/xdg \
    OZY_SIDECAR_SOURCE=/opt/ozy/sidecar \
    OZY_SIDECAR_PYTHON=/usr/bin/python3

# Minimal semantic-on config used only to warm the sidecar at build time.
RUN printf '%s\n' '{ "search": { "semantic": { "enabled": true } }, "embedding": { "model": "BAAI/bge-small-en-v1.5", "vectorBackend": "turbovec" } }' > /bench/warm.jsonc

# Warm: provision the venv (python -m venv + pip fastembed/turbovec) AND download
# the model (the doctor readiness probe loads it) into
# $XDG_STATE_HOME/ozy/sidecar/venv/{,models}. Provisioning streams the pip/venv
# subprocess output to stdout, which would pollute a `--format json` capture, so
# this warm run's output is discarded; the gate below re-checks on a clean run.
RUN OZY_CONFIG=/bench/warm.jsonc ozy doctor >/dev/null 2>&1 || true

# Gate: a second doctor hits the just-written provision marker + cached model, so
# it reprovisions nothing and its JSON stdout is clean. `ozy doctor` always exits
# 0, so we gate on the JSON: a non-OK embedding check fails the build — a run must
# never silently fall back to lexical.
RUN OZY_CONFIG=/bench/warm.jsonc ozy doctor --format json > /tmp/doctor-warm.json 2>/tmp/doctor-warm.err; \
    python3 -c "import json,sys; c=[x for x in json.load(open('/tmp/doctor-warm.json'))['checks'] if x['name']=='embedding']; sys.exit(0 if c and c[0]['status']=='ok' else 1)" \
      || (echo 'BAKE FAILURE: semantic embedder not ready at build time (venv/model provisioning failed)' >&2; echo '--- doctor stdout ---' >&2; cat /tmp/doctor-warm.json >&2; echo '--- doctor stderr ---' >&2; cat /tmp/doctor-warm.err >&2; exit 1)

# From here the model MUST NOT be re-fetched at run time.
ENV HF_HUB_OFFLINE=1 \
    TRANSFORMERS_OFFLINE=1

# Hermeticity smoke: with NO network, the baked venv+model must still yield a
# SEMANTIC embedding check (marker hit → no reprovision, cached model → no fetch).
# A lexical fallback or any fetch attempt fails the build.
RUN --network=none OZY_CONFIG=/bench/warm.jsonc ozy doctor --format json > /tmp/doctor-offline.json 2>/tmp/doctor-offline.err; \
    python3 -c "import json,sys; c=[x for x in json.load(open('/tmp/doctor-offline.json'))['checks'] if x['name']=='embedding']; sys.exit(0 if c and c[0]['status']=='ok' else 1)" \
      || (echo 'HERMETICITY FAILURE: semantic not available offline — baked venv/model missing or incomplete' >&2; echo '--- doctor stdout ---' >&2; cat /tmp/doctor-offline.json >&2; echo '--- doctor stderr ---' >&2; cat /tmp/doctor-offline.err >&2; exit 1)

# Retrieval-stack provenance: ozy exercises semantic retrieval (embedder baked
# above, runtime offline). resolveRetrievalStack() reads this label as-is.
ENV OZY_BENCH_RETRIEVAL=semantic
WORKDIR /bench

ENTRYPOINT ["ozy-bench", "run"]
