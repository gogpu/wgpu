# Contributing to wgpu

Thank you for your interest in contributing to the Pure Go WebGPU implementation!

wgpu is part of the [gogpu](https://github.com/gogpu) ecosystem — 1.1M+ lines of Pure Go GPU code powering 2D graphics, 3D rendering, GUI toolkit, ML frameworks, and a game engine.

## AI-Assisted Contributions: Smart Coding Welcome

**We welcome AI-assisted contributions.** This entire ecosystem is built using AI-assisted workflows, and we don't consider AI assistance a negative signal. What matters is the quality of the result, not the tool used to produce it.

We practice [**Smart Coding**](https://dev.to/kolkov/from-vibe-coding-to-agentic-engineering-what-karpathy-got-right-and-whats-missing-62e) — a framework where human engineering judgment drives AI capabilities. The key insight: **you own architecture, AI owns implementation**. The 70/30 rule applies — spend 70% of effort on architecture, review, and validation; 30% on implementation.

There are three paradigms on the AI-assisted development spectrum:

| | Vibe Coding | Agentic Engineering | Smart Coding |
|---|---|---|---|
| **Approach** | "Give in to the vibes, forget code exists" (Karpathy) | Orchestrate AI agents with specs and quality gates | Meta-framework: engineering judgment decides when to explore vs. build |
| **Strengths** | Fast prototyping, feasibility spikes | Parallel workflows, specification-driven | Knowledge compounds across sessions, bidirectional learning |
| **Weakness** | Production disasters without oversight | "Like conducting an orchestra that can't remember yesterday's piece" | Requires experienced engineers who understand the domain |
| **Our verdict** | Good for exploration, never for production | Good foundation, missing feedback loop | **This is what we practice** |

**What separates Smart Coding from low-quality AI-generated code:**

| Smart Coding | Low-Quality AI Code |
|---|---|
| Understands the reference implementation (Rust wgpu) | Copy-pastes without understanding the architecture |
| Researches before coding — consults enterprise references | "Insert call and see what happens" |
| Tests on real hardware and real examples | "Build passes, ship it" |
| Handles edge cases and error paths | Happy path only |
| Can explain design decisions when asked | "The AI suggested it" |
| Clean commit history with meaningful messages | Single 10K-line commit with "add feature" |
| Iterates on review feedback with understanding | Regenerates entire file and hopes for the best |

**What we look for in any contribution, AI-assisted or not:**

- Evidence that you understand what the code does and why
- Rust wgpu reference consulted for non-trivial changes (we port from [gfx-rs/wgpu](https://github.com/gfx-rs/wgpu))
- Tests that verify behavior, not just compilation
- Willingness to iterate on review feedback

**We do NOT require:**

- Disclosure of AI tool usage — it's your choice
- "Hand-written" code — we care about correctness, not process
- Perfect first submission — we'll work with you to get it right

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/wgpu`
3. Create a branch: `git checkout -b feat/your-feature`
4. Make your changes
5. Run checks (see below)
6. Commit using [Conventional Commits](https://www.conventionalcommits.org/)
7. Push: `git push origin feat/your-feature`
8. Open a Pull Request

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

Run these before pushing:

```bash
go fmt ./...                      # Format
go vet ./...                      # Vet
golangci-lint run --timeout=5m    # Lint
go build ./...                    # Build
go test ./...                     # Test
```

For platform-specific changes, lint on all target platforms:

```bash
GOOS=linux GOARCH=amd64 golangci-lint run --timeout=5m
GOOS=darwin GOARCH=arm64 golangci-lint run --timeout=5m
```

Platform-specific files (`_darwin.go`, `_linux.go`, `_windows.go`) are invisible to lint on other platforms.

## Project Structure

```
wgpu/
├── *.go                # Public API (20 types wrapping core/ and hal/)
├── core/               # Validation, resource management, state tracking
│   └── track/          # Buffer/resource state tracking
├── hal/                # Hardware abstraction layer (interfaces + descriptors)
│   ├── vulkan/         # Vulkan backend (Windows, Linux, macOS, Android)
│   ├── metal/          # Metal backend (macOS, iOS)
│   ├── dx12/           # DirectX 12 backend (Windows)
│   ├── gles/           # OpenGL ES backend (Windows, Linux)
│   ├── software/       # Software rasterizer + SPIR-V interpreter
│   ├── noop/           # No-op backend (testing)
│   └── allbackends/    # Backend registration convenience
├── internal/           # Internal utilities
├── cmd/                # CLI tools and test apps
└── examples/           # Example applications
```

## Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(vulkan): add Android arm64 WSI support
fix(metal): pin autorelease pools to OS threads
docs: update ARCHITECTURE.md
test(core): add surface lifecycle tests
refactor(hal): share checked swapchain enumeration
chore: bump dependencies
```

Components: `core`, `hal`, `vulkan`, `metal`, `dx12`, `gles`, `software`, `noop`, `docs`, `ci`

## Pull Request Guidelines

- Keep PRs focused on a single change
- Reference the Rust wgpu equivalent for non-trivial HAL/core changes
- Add tests for new functionality
- Update documentation if needed
- Ensure all CI checks pass
- Reference related issues

For large features, consider splitting into a prerequisite stack — small, reviewable PRs that build on each other. We take responsibility for every line that lands in the codebase.

## Testing

```bash
go test ./...              # Unit tests
go test -cover ./...       # With coverage
go test -race ./...        # With race detector (requires CGO_ENABLED=1)
```

For backend-specific verification, run examples with backend selection:

```bash
GOGPU_GRAPHICS_API=vulkan   go run ./examples/compute-sum/
GOGPU_GRAPHICS_API=dx12     go run ./examples/compute-sum/
GOGPU_GRAPHICS_API=software go run ./examples/compute-sum/
```

## Reporting Issues

- Use [GitHub Issues](https://github.com/gogpu/wgpu/issues)
- Include Go version, OS, and GPU (if relevant)
- Provide minimal reproduction
- Include error messages and backend used (`GOGPU_GRAPHICS_API=?`)

## Questions?

- [GitHub Discussions](https://github.com/orgs/gogpu/discussions) for questions and ideas
- PR comments for code-specific discussion

---

Thank you for contributing!
