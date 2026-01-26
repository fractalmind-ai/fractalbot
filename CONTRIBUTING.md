# Contributing to FractalBot

Thank you for your interest in contributing to FractalBot! This document provides guidelines and instructions for contributing to the project.

## 📋 How to Contribute

### Reporting Bugs

Before creating bug reports, please check the existing issues as you might find that the problem has already been reported. When creating a bug report, please include:

- A clear and descriptive title
- Steps to reproduce the issue
- Expected behavior vs. actual behavior
- Environment details (OS, Go version, channel type)
- Any relevant logs or error messages

### Suggesting Enhancements

Enhancement suggestions are welcome! Please:

- Use a clear and descriptive title
- Provide a detailed explanation of the suggested enhancement
- Explain why this enhancement would be useful
- Provide examples if applicable

### Pull Requests

Pull requests are welcome! To ensure your PR is accepted:

1. Fork the repository
2. Create a new branch for your feature (`git checkout -b feature/amazing-feature`)
3. Make your changes and write clear commit messages
4. Ensure your code follows Go best practices
5. Add tests for new features
6. Update documentation as needed
7. Push to your branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

## 🛠️ Development Setup

```bash
# Clone your fork
git clone git@github.com:your-username/fractalbot.git
cd fractalbot

# Install dependencies
go mod download

# Build
go build -o fractalbot ./cmd/fractalbot

# Run tests
go test ./...
```

## 📝 Coding Standards

- Follow [Effective Go](https://go.dev/doc/effective_go) guidelines
- Use `gofmt` for formatting (`gofmt -s -w .`)
- Run `go vet` before committing (`go vet ./...`)
- Add comments for exported functions and types
- Keep functions small and focused

## 🧪 Testing

Before submitting a PR, ensure:

- All tests pass (`go test ./...`)
- No new linting errors (`go vet ./...`)
- Code is formatted (`gofmt -l .` returns nothing)
- Changes are well-tested with appropriate test coverage

## 📖 Documentation

- Keep documentation up-to-date
- Add godoc comments to exported functions
- Update README.md if needed
- Document any breaking changes

## 💬 Communication

- Be respectful and constructive
- Ask questions if anything is unclear
- Participate in discussions on issues and PRs
- Follow the FractalMind AI philosophy of process-oriented development

## 📄 License

By contributing, you agree that your contributions will be licensed under the MIT License.

## 🎮 FractalMind Principles

This project follows FractalMind AI's philosophy:

- **Process-Oriented**: Strict workflows and quality gates
- **Anti-Drift**: Clear scope boundaries and exploration budgets
- **Evidence-Based**: Always provide evidence of changes
- **Team Coordination**: Multi-agent collaboration with lead orchestration

## 📚 Resources

- [Go Documentation](https://go.dev/doc/)
- [Effective Go](https://go.dev/doc/effective_go)
- [Clawdbot](https://github.com/clawdbot/clawdbot) - Inspiration source
- [FractalMind AI](https://github.com/fractalmind-ai) - Organization
