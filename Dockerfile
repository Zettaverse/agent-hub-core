# syntax=docker/dockerfile:1

# ---------- Stage 1: build the Rust MCP engine (libmcpengine cdylib) ----------
FROM rust:1.96 AS rust-builder
WORKDIR /src
# The Rust engine is vendored as a git submodule at third_party/mcp-engine
# (checked out recursively by CI before the Docker build).
COPY third_party/mcp-engine /src/mcp-engine
WORKDIR /src/mcp-engine
RUN cargo build --release

# ---------- Stage 2: build the Go binary with cgo against libmcpengine ----------
FROM golang:1.26 AS go-builder
WORKDIR /src

# Copy the Rust cdylib and header into the expected layout
# (third_party/mcp-engine/{include,target/release}).
COPY --from=rust-builder /src/mcp-engine/include /src/third_party/mcp-engine/include
COPY --from=rust-builder /src/mcp-engine/target/release /src/third_party/mcp-engine/target/release

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=1
RUN go build -tags mcpengine -ldflags="-s -w" -o /out/hub ./cmd/hub

# ---------- Stage 3: minimal runtime image ----------
FROM gcr.io/distroless/base-debian12:nonroot
COPY --from=go-builder /out/hub /hub
EXPOSE 8080
ENTRYPOINT ["/hub"]
