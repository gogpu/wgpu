# Contributing to wgpu

Thank you for your interest in contributing to the Pure Go WebGPU implementation!

wgpu is part of the [GoGPU](https://github.com/gogpu) ecosystem of Pure Go GPU libraries: 2D graphics, 3D rendering, a GUI toolkit, and machine learning frameworks that build on it.

## Current Priorities

See [tracking issues](https://github.com/gogpu/wgpu/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22) for what we need help with. Key areas:

- **#329** — Move all implementation to `internal/` (architecture cleanup)
- **#360** — `EnumerateAdapters` on the public `wgpu.Instance`
- **#218** — Public API cleanup before v1.0

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

New code goes into `internal/` packages. The root package contains only exported types and thin delegation methods. See [ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Project Structure

```
wgpu/
├── *.go                # Public API (exported types + thin wrappers)
├── core/               # Validation, resource management, state tracking
├── hal/                # Hardware abstraction layer
│   ├── vulkan/         # Vulkan backend (Windows, Linux, macOS, Android preview)
│   ├── metal/          # Metal backend (macOS)
│   ├── dx12/           # DirectX 12 backend (Windows)
│   ├── gles/           # OpenGL / OpenGL ES backend (Windows, Linux)
│   ├── software/       # Full CPU rasterizer + SPIR-V interpreter + RT BVH
│   └── noop/           # No-op backend (testing)
├── internal/
│   ├── browser/        # Browser WebGPU via syscall/js
│   ├── raytracing/     # RT build orchestration, compaction, validation
│   └── ...             # indirect, pipelinecache, shaderir, thread
├── cmd/                # CLI tools and test apps
├── examples/           # Example applications
│   ├── raytracing-headless/  # software HAL BLAS build, rays traced on the CPU
│   ├── compute-sum/          # Compute shader example
│   └── ...
└── docs/               # Public documentation
```

> **Note:** `core/` and `hal/` are planned to move under `internal/` (#329). New implementation code should go into `internal/` sub-packages.

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
- Ensure all CI checks pass (build across three OSes and windows/386, cross-compile for browser and the Rust binding, tests with the race detector, GLES integration, lint on windows/amd64 and darwin/arm64 as well as linux, formatting, deps, Android preview)
- Reference related issues

## Testing

```bash
go test ./...              # Unit tests
go test -cover ./...       # With coverage
go test -race ./...        # Race detector (needs CGO_ENABLED=1 and a C compiler; CI uses the pure-Go github.com/kolkov/racedetector)
```

Backend-specific verification:

```bash
go run ./examples/compute-sum/           # runs on the best available backend
go run ./examples/raytracing-headless/   # software HAL BLAS build (no GPU needed)
go test ./hal/allbackends/               # cross-backend readback tests on every adapter found
```

## Reporting Issues

- Use [GitHub Issues](https://github.com/gogpu/wgpu/issues)
- Include Go version, OS, and GPU (if relevant)
- Provide minimal reproduction
- Include error messages and the backend used (Vulkan, DX12, Metal, GLES, Software)

## Questions?

- [GitHub Discussions](https://github.com/orgs/gogpu/discussions) for questions and ideas
- [W3C gpuweb discussion](https://github.com/gpuweb/gpuweb/discussions/7023) for WebGPU spec topics
- PR comments for code-specific discussion

---

Thank you for contributing!
