# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.x.x   | :white_check_mark: |

## Reporting a Vulnerability

To report a security vulnerability:

1. **Do NOT** open a public issue
2. Email security concerns to the maintainers via GitHub private vulnerability reporting
3. Or open a private security advisory at: https://github.com/gogpu/wgpu/security/advisories/new

We will respond within 48 hours and work with you to understand and address the issue.

## Security Considerations

wgpu is a WebGPU implementation that interfaces with GPU hardware. Security considerations include:

- **Memory safety**: Pure Go implementation minimizes unsafe code
- **Resource limits**: Proper validation of buffer sizes and texture dimensions
- **Shader validation**: Input shaders should be validated before execution
- **Platform integration**: Backend-specific code follows platform security guidelines

## Disclosure Policy

We follow responsible disclosure practices:

1. Reporter notifies us privately
2. We acknowledge and investigate
3. We develop and test a fix
4. We release the fix and credit the reporter (if desired)
5. We publish a security advisory
