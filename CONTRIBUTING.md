# Contributing to ShipItAI

Thank you for your interest in contributing to ShipItAI! This document provides guidelines and instructions for contributing.

## Getting Started

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR-USERNAME/shipitai.git
   cd shipitai
   ```
3. Install dependencies:
   ```bash
   make deps
   ```
4. Run tests to verify your setup:
   ```bash
   make test
   ```

## Development Workflow

1. Create a feature branch from `main`:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. Make your changes, following the code style guidelines below

3. Write or update tests for your changes

4. Run the test suite:
   ```bash
   make test
   ```

5. Run the linter:
   ```bash
   make lint
   ```

6. Commit your changes with a clear commit message

7. Push to your fork and submit a pull request

## Code Style

- Run `go fmt` before committing
- Follow existing patterns in the codebase
- Keep functions focused and reasonably sized
- Add comments for non-obvious logic
- Use meaningful variable and function names

## Testing

- Write unit tests for new functionality
- Ensure all tests pass before submitting a PR
- Aim for good test coverage on critical paths

Run tests:
```bash
make test
```

Generate coverage report:
```bash
make test-coverage
```

## Pull Request Process

1. Update documentation if needed (README, code comments, etc.)
2. Add tests for new functionality
3. Ensure CI passes (tests, lint)
4. Request review from maintainers
5. Address review feedback

### PR Title Format

Use a clear, descriptive title:
- `Add support for custom review templates`
- `Fix comment parsing for multi-line strings`
- `Improve chunking performance for large diffs`

### PR Description

Include:
- Summary of changes
- Motivation/context
- Testing performed
- Breaking changes (if any)

## Reporting Issues

### Bug Reports

Include:
- Clear description of the bug
- Steps to reproduce
- Expected vs actual behavior
- Environment details (Go version, OS, etc.)
- Relevant logs (redact any secrets!)

### Feature Requests

Include:
- Problem you're trying to solve
- Proposed solution
- Alternative approaches considered
- Willingness to implement

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you agree to uphold this code.

## Questions?

- Open a GitHub Discussion for general questions
- Open an Issue for bugs or feature requests

Thank you for contributing!
