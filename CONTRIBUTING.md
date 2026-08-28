# Contributing to wgpu

Thank you for your interest in contributing to the Pure Go WebGPU implementation!

wgpu is part of the [GoGPU](https://github.com/gogpu) ecosystem — 1.25M+ lines of Pure Go GPU code powering 2D graphics, 3D rendering, GUI toolkit, ML frameworks, and game engines.

## Current Priorities

See [tracking issues](https://github.com/gogpu/wgpu/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22) for what we need help with. Key areas:

- **#329** — Move all implementation to `internal/` (architecture cleanup, P0)
- **#330** — Expose CreateQuerySet + CreateRenderBundleEncoder (good first issue)
- **#331** — Pipeline cache for Vulkan + DX12 (performance)
- **#333** — Validation Phase C (~45%→~70% coverage)
- **#334** — SPIR-V interpreter missing opcodes

## AI-Assisted Contributions: Smart Coding Welcome

**We welcome AI-assisted contributions.** This entire ecosystem is built using AI-assisted workflows. What matters is the quality of the result, not the tool used.

We practice **Smart Coding** — human engineering judgment drives AI capabilities. The key: **you own architecture, AI owns implementation**. 70% effort on architecture/review/validation, 30% on implementation.

**What we look for in any contribution:**

- Evidence that you understand what the code does and why
- Rust wgpu reference consulted for non-trivial changes (we port from [gfx-rs/wgpu](https://github.com/gfx-rs/wgpu))
- Tests that verify behavior, not just compilation
- Willingness to iterate on review feedback

**We do NOT require:**

- Disclosure of AI tool usage
- "Hand-written" code — we care about correctness
- Perfect first submission — we'll work with you

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/wgpu`
3. Create a branch: `git checkout -b feat/your-feature`
4. Make your changes
5. Run checks (see below)
6. Commit using [Conventional Commits](https://www.conventionalcommits.org/)
7. Push and open a Pull Request

## Development Setup

```bash
git clone https://github.com/gogpu/wgpu
cd wgpu

go mod download
go build ./...
go test ./...
golangci-lint run --timeout=5m
```

**Requirements:** Go 1.25+, `golangci-lint`, `CGO_ENABLED=0` (pure Go, no C compiler needed).

## Pre-Submit Checklist

```bash
go fmt ./...                      # Format
go vet ./...                      # Vet
golangci-lint run --timeout=5m    # Lint
go build ./...                    # Build
go test ./...                     # Test
```

For platform-specific changes, lint on **all target platforms:**

```bash
GOOS=linux GOARCH=amd64 golangci-lint run --timeout=5m
GOOS=darwin GOARCH=arm64 golangci-lint run --timeout=5m
```

Platform-specific files (`_darwin.go`, `_linux.go`, `_windows.go`) are invisible to lint on other platforms.

## Architecture Principle

**If implementation CAN be hidden — it MUST be hidden.**

New code goes into `internal/` packages. The root package contains only exported types and thin delegation methods. See [ARCHITECTURE.md](docs/ARCHITECTURE.md) and ADR-070.

## Project Structure

```
wgpu/
├── *.go                # Public API (exported types + thin wrappers)
├── internal/
│   ├── core/           # Validation, resource management, state tracking
│   ├── hal/            # Hardware abstraction layer
│   │   ├── vulkan/     # Vulkan backend (Windows, Linux, macOS, Android)
│   │   ├── metal/      # Metal backend (macOS, iOS)
│   │   ├── dx12/       # DirectX 12 backend (Windows)
│   │   ├── gles/       # OpenGL ES backend (Windows, Linux)
│   │   ├── software/   # Full CPU rasterizer + SPIR-V interpreter + RT BVH
│   │   └── noop/       # No-op backend (testing)
│   ├── browser/        # Browser WebGPU via syscall/js
│   ├── raytracing/     # RT build orchestration, compaction, validation
│   └── ...
├── cmd/                # CLI tools and test apps
├── examples/           # Example applications
│   ├── raytracing-headless/  # RT verification on software backend
│   ├── compute-sum/          # Compute shader example
│   └── ...
└── docs/               # Public documentation
```

> **Note:** Migration from `core/` and `hal/` to `internal/` is in progress (#329). New code should go directly into `internal/` sub-packages.

## Commit Messages

[Conventional Commits](https://www.conventionalcommits.org/):

```
feat(vulkan): add Android arm64 WSI support
fix(metal): pin autorelease pools to OS threads
docs: update ARCHITECTURE.md
test(core): add surface lifecycle tests
refactor(hal): share checked swapchain enumeration
```

Components: `core`, `hal`, `vulkan`, `metal`, `dx12`, `gles`, `software`, `noop`, `browser`, `raytracing`, `docs`, `ci`

## Pull Request Guidelines

- Keep PRs focused on a single change
- Reference the Rust wgpu equivalent for non-trivial HAL/core changes
- Add tests for new functionality
- Update documentation if needed (CHANGELOG, docs/, README)
- Ensure all CI checks pass (14 checks: build×3, test×3, lint, format, deps, GLES integration, browser, Android, Rust cross-compile)
- Reference related issues

## Testing

```bash
go test ./...              # Unit tests
go test -cover ./...       # With coverage
go test -race ./...        # Race detector (requires CGO_ENABLED=1)
```

Backend-specific verification:

```bash
GOGPU_GRAPHICS_API=vulkan   go run ./examples/compute-sum/
GOGPU_GRAPHICS_API=dx12     go run ./examples/compute-sum/
GOGPU_GRAPHICS_API=software go run ./examples/compute-sum/
go run ./examples/raytracing-headless/   # RT verification (no GPU needed)
```

## Reporting Issues

- Use [GitHub Issues](https://github.com/gogpu/wgpu/issues)
- Include Go version, OS, and GPU (if relevant)
- Provide minimal reproduction
- Include error messages and backend used (`GOGPU_GRAPHICS_API=?`)

## Questions?

- [GitHub Discussions](https://github.com/orgs/gogpu/discussions) for questions and ideas
- [W3C gpuweb discussion](https://github.com/gpuweb/gpuweb/discussions/7023) for WebGPU spec topics
- PR comments for code-specific discussion

---

Thank you for contributing!
