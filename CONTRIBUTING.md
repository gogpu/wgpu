# Contributing to wgpu

Thank you for your interest in contributing to wgpu!

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/wgpu`
3. Create a branch: `git checkout -b feat/your-feature`
4. Make your changes
5. Run tests: `go test ./...`
6. Commit: `git commit -m "feat: add your feature"`
7. Push: `git push origin feat/your-feature`
8. Open a Pull Request

## Development Setup

```bash
# Clone the repository
git clone https://github.com/gogpu/wgpu
cd wgpu

# Install dependencies
go mod download

# Run tests
go test ./...

# Run linter
golangci-lint run
```

## Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Use `golangci-lint` for linting
- Write tests for new functionality
- Document public APIs

## Project Structure

```
wgpu/
├── types/          # WebGPU type definitions
├── core/           # Core validation and state tracking
├── hal/            # Hardware abstraction layer
│   ├── vulkan/     # Vulkan backend
│   ├── metal/      # Metal backend
│   ├── dx12/       # DirectX 12 backend
│   └── gl/         # OpenGL backend
├── internal/       # Internal utilities
└── cmd/            # CLI tools (if any)
```

## Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(component): add new feature
fix(component): fix bug
docs: update documentation
test: add tests
refactor: code refactoring
chore: maintenance tasks
```

Components: `types`, `core`, `hal`, `vulkan`, `metal`, `dx12`, `gl`, `docs`, `ci`

## Pull Request Guidelines

- Keep PRs focused on a single change
- Update documentation if needed
- Add tests for new features
- Ensure all tests pass
- Reference related issues

## Testing

### Unit Tests
```bash
go test ./...
```

### With Coverage
```bash
go test -cover ./...
```

### With Race Detector
```bash
go test -race ./...
```

## Reporting Issues

- Use GitHub Issues
- Include Go version and OS
- Provide minimal reproduction
- Include error messages

## Questions?

Open a GitHub Discussion or reach out to maintainers.

---

Thank you for contributing!
