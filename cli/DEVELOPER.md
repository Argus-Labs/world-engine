# World CLI Developer Guide

This guide provides information for developers who want to contribute to the World CLI project.

## Prerequisites

- [Go 1.27.1](https://go.dev/doc/install) or later
- [Moonrepo](https://moonrepo.dev/docs/install) - Build system used for build, test, and lint commands
- [Docker](https://docs.docker.com/get-docker/) - Required for running services managed by World CLI
- [Docker Compose](https://docs.docker.com/compose/install/) - Required for multi-container Docker applications
- **Windows Users:** Windows Subsystem for Linux 2 (WSL2) is required for running World CLI on Windows

## Getting Started

### Clone the Repository

World CLI lives in the `cli/` directory of the World Engine repository and is part of its Go module. Clone it:

```bash
git clone https://github.com/Argus-Labs/world-engine.git
cd world-engine/cli
```

### Setup Development Environment

The World CLI uses Moonrepo as a task runner. Tasks live in the `cli` project; run them from anywhere in the repository:

```bash
moon run cli:build   # Build all packages
moon run cli:test    # Unit tests
moon run cli:lint    # golangci-lint with cli/.golangci.yaml
```

Install [buf](https://buf.build/docs/installation) to regenerate protobuf code.

## Development Workflow

### Project Structure

The World CLI is organized around several core systems:

- `cmd/world/` - Contains the main CLI command implementations
  - `main.go` - Entry point for the CLI application
  - `root/` - Root command and core commands like `create` and `doctor`
  - `cardinal/` - Cardinal game shard management commands
  - `evm/` - EVM-related commands
- `common/` - Shared utility code used across the application
  - `config/` - Configuration loading and management
  - `docker/` - Docker client and service definitions
  - `globalconfig/` - Global configuration persistence
  - `logger/` - Logging utilities
  - `teacmd/` - Terminal UI command utilities
- `proto` - Contain proto files for Connect RPC
- `tea/` - Terminal UI components using Bubble Tea framework
  - `component/` - Reusable UI components
  - `style/` - Terminal styling utilities
- `telemetry/` - Telemetry integration for error tracking and analytics
- `moon.yml` - Moon task definitions for building, testing, and installing World CLI
- `example-world.toml` - Example configuration file

### Generating Connect and PB files

World CLI talks to the Cardinal Operator over [Connect RPC](https://connectrpc.com/docs/introduction). The API lives in `proto/cardinal/operator/v1`. To regenerate the Go code:

```bash
moon run cli:generate
```

### Running the CLI from Source

```bash
go run ./cmd/world --help
```

To try your local checkout inside a game project, point the project at it:

```bash
go mod edit -replace github.com/argus-labs/world-engine/cli=/path/to/world-engine/cli
go tool world --help
```

Drop the `replace` before committing the game project.

## Configuration

The World CLI uses a TOML configuration file to manage settings for various services. An example configuration file is provided in the repository: `example-world.toml`.

Key sections in the configuration file include:

- `[cardinal]` - Settings for the Cardinal game shard
- `[evm]` - Settings for the Ethereum Virtual Machine
- `[common]` - Common settings shared across components
- `[nakama]` - Settings for the Nakama game server

Create a `world.toml` file in your project directory based on the example:

```bash
cp example-world.toml world.toml
```

Then customize the settings as needed for your development environment.

## Testing

### Running Tests

World CLI has both unit and integration tests:

```bash
moon run cli:test              # Unit tests
moon run cli:test-integration  # Integration tests (requires Docker)
moon run cli:test-ci           # Unit tests with coverage (coverage.out)
```

## Linting

### Running Linter

```bash
moon run cli:lint
```

## Pull Request Process

1. Create a feature branch from the `main` branch
2. Make your changes
3. Ensure tests pass: `moon run cli:test`
4. Ensure linting passes: `moon run cli:lint`
5. Push your changes and create a pull request
6. PR titles must follow the [conventional commit](https://www.conventionalcommits.org/) format
7. All tests and linting checks must pass with adequate coverage

## Additional Tools

### TUI Development

The World CLI uses the following libraries for terminal user interface development:

- [bubbletea](https://github.com/charmbracelet/bubbletea) - A framework for building terminal apps
- [lipgloss](https://github.com/charmbracelet/lipgloss) - Style definitions for terminal applications

### Docker Services

The CLI manages several Docker services:

1. **Cardinal**: Core game shard service
2. **Cardinal Editor**: Development tool for Cardinal
3. **Nakama**: Game server for multiplayer functionality
4. **NakamaDB**: Database for Nakama
5. **Redis**: In-memory data store
6. **EVM**: Ethereum Virtual Machine chain
7. **Celestia DevNet**: Data Availability layer for EVM
8. **Jaeger**: Distributed tracing (optional)
9. **Prometheus**: Metrics collection (optional)

## Troubleshooting

### Doctor Command

The World CLI includes a `doctor` command to check your system for dependencies and configuration issues:

```bash
world doctor
```

This command checks for required tools and services, and provides guidance on fixing any issues found.
