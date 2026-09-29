<div align="center"> <!-- markdownlint-disable-line first-line-heading -->
<img alt="World CLI Logo" src="https://i.imgur.com/XM74ODi.png" width="378">
<p>A swiss army knife for creating, managing, and deploying World Engine projects</p>
  <p>
    <a href="https://codecov.io/gh/Argus-Labs/world-cli" >
    <img alt="Codecov" src="https://codecov.io/gh/Argus-Labs/world-cli/branch/main/graph/badge.svg?token=XMH4P082HZ"/>
    </a>
    <a href="https://goreportcard.com/report/pkg.world.dev/world-cli">
    <img alt="Go Report Card" src="https://goreportcard.com/badge/pkg.world.dev/world-cli">
    </a>
    <a href="https://t.me/worldengine_dev" target="_blank">
    <img alt="Telegram Chat" src="https://img.shields.io/endpoint?color=neon&logo=telegram&label=chat&url=https%3A%2F%2Ftg.sumanjay.workers.dev%2Fworldengine_dev">
    </a>
    <a href="https://x.com/WorldEngineGG" target="_blank">
    <img alt="Twitter Follow" src="https://img.shields.io/twitter/follow/WorldEngineGG">
    </a>
  </p>
</div>

## Overview

World CLI is a comprehensive command-line tool for managing World Engine projects. It provides everything you need to create, develop, deploy, and manage your World Engine games and applications.

### Key Features

- **Project Management** — Create new World Engine projects from templates
- **Development Tools** — Run Cardinal game shards in development mode with hot reloading
- **Cloud Deployment** — Deploy your projects to World Engine cloud infrastructure
- **User Management** — Manage organizations, projects, and team members
- **Health Monitoring** — Check project status and view logs

**Need help getting started with World Engine?** Check out the [World Engine docs](https://world.dev)!

<br/>

## Installation

Before installing World CLI, you'll need to have Go installed on your system.
If you haven't installed Go yet, follow the official [Go installation guide](https://go.dev/doc/install) to get started.

**Windows Users:** Windows Subsystem for Linux 2 (WSL2) is required for running World CLI on Windows.

### World CLI Installation

**Download from GitHub Releases:**

Visit [https://github.com/Argus-Labs/world-cli/releases](https://github.com/Argus-Labs/world-cli/releases) to download the latest release for your platform.

**Linux:**
```shell
curl -L https://github.com/Argus-Labs/world-cli/releases/download/{@latest}/world-cli_Linux_x86_64.tar.gz | tar -xz
sudo mv world /usr/local/bin/
```

**macOS:**
```shell
curl -L https://github.com/Argus-Labs/world-cli/releases/download/{@latest}/world-cli_Darwin_x86_64.tar.gz | tar -xz
sudo mv world /usr/local/bin/
```

**Windows:**
```powershell
Invoke-WebRequest -Uri "https://github.com/Argus-Labs/world-cli/releases/download/{@latest}/world-cli_Windows_x86_64.zip" -OutFile "world-cli.zip"
Expand-Archive -Path "world-cli.zip" -DestinationPath "$env:USERPROFILE\go\bin" -Force
Remove-Item "world-cli.zip"
```

<br/>

## Getting Started

### 1. Authentication

First, authenticate with World Engine:

```shell
world login
```

This will open your browser to complete the authentication process.

### 2. Setup Your First Project

Setup a new World Engine project:

```shell
world setup
```

This will guide you through setting up a new project with the starter template.

### 3. Run your project

Start your project:

```shell
world start
```

This runs your Cardinal game shard.

<br/>

## Available Commands

- **`world setup`** — Initialize a new World Engine project from templates
- **`world doctor`** — Check that all required dependencies are installed
- **`world version`** — Display version information and check for updates

### SDK Generation

- **`world sdk generate <source> [--go-out DIR] [--cs-out DIR] [--proto-out DIR]`** — Generate typed, reflection-free wire serialization (Go + C#) for a backend's commands, events, components, and system events.

Discovers commands, events, components, and system events across the backend's import graph, emits a `.proto`, and runs `buf`/protoc in Docker to produce Go proto structs (plus an in-place `wire.gen.go` carrying the `schema.Serializable` wire methods per package) and C# classes. Requires Docker (same as `world start`). Pass `--go-out` and/or `--cs-out` (at least one). `<source>` may be a local backend dir or a remote git URL/`owner/repo` (cloned at `--ref`).

Discovery is by registration: a command via `w.RegisterCommand[T]()` or a `w.SendToShard` call, an event via `w.RegisterEvent[T]()`, a component via `w.RegisterComponent[T]()`, a system event via `w.RegisterSystemEvent[T]()`. A type that's declared but never registered is not emitted; one that cardinal uses without a registration blocks generation. The generated wire methods live on the type itself, so a type that hasn't been generated is missing them and the shard **fails to compile** — a build error, never a silent wire failure at runtime.

`--go-out` writes the wire code **back into the backend's own packages** (Go requires a method's receiver type to be same-package), so it requires a local checkout — a remote source is read-only and generates the client SDK (`--cs-out`) only. To keep the backend and client in lockstep, pass both in a single run: one discovery, one `buf` run, both outputs generated from the same source state. `--proto-out DIR` (**debugging only, slated for deprecation**) additionally persists the `.proto` schema so the wire contract can be read by hand; it requires `--go-out`. The export carries only this backend's own files — a dependency's schema is dropped while the `import` line naming it is kept — so it does not compile standalone and is not a `buf breaking` input. Nothing in generation or CI reads it: buf's module is the hand-written `proto/` tree, and the backends that once persisted a copy keep none. Use it to inspect the schema, not to build anything on.

**Generation report.** Every run prints a report grouping issues by category. Every category blocks — nothing is generated until they are fixed — and each names its fix:

- `duplicate-name` — the same proto message name in two packages → consolidate to one shared type.
- `duplicate-wire-name` — two commands share a `Name()` → give each a distinct name.
- `unserializable` — a `chan`/`func`/`unsafe.Pointer`/method-interface field, or a struct with no exported fields → remove it from the wire type, or send an ID the other side resolves.
- `slice` — `[]T` or `[]byte` → give it a bound: `[N]T` generates a typed `repeated` field and `[N]byte` becomes proto `bytes`.
- `map` — `map[K]V` → a struct with one field per key if the key set is fixed, otherwise a fixed array of entry structs plus a count.
- `pointer` — `*T` or an optional scalar → drop the star and put presence in the data.
- `interface` — `any`/`interface{}` → the one concrete type it holds, or a tag field plus one field per variant.
- `unsupported-type` — data this generator has no mapping for (`uintptr`, `complex`) → a scalar, a string, or a named struct of those.

A field holding a reference is reported under its shape because the bytes of the field are not the whole value: an ECS column copy would share memory with the live world, and the schema states no bound for it.

**A type from another module** is not a violation on its own — the generator rebuilds it into your schema (mirrored) or imports it when its owner ran the generator. But its fields are judged by the rules above, all the way down, and you cannot edit them. When a dependency's own field shapes block you, the fix is a local struct that carries only what the wire needs:

```go
// ✗ blocks: net.TCPAddr mirrors fine, but its IP field is a []byte — reported as `slice` on TCPAddr.IP
type MoveCommand struct {
    Target net.TCPAddr
}

// ✓ a local type with wire-shaped fields; convert at the boundary
type Endpoint struct {
    IP   [16]byte
    Port int32
}

type MoveCommand struct {
    Target Endpoint
}
```

**Generate for the rampage project** (run from anywhere; assumes the repos are siblings):

```shell
# Base dir where the repos are checked out.
ROOT=~/go/src/github.com/Argus-Labs

world sdk generate \
  "$ROOT/rampage-backend" \
  --go-out "$ROOT/rampage-backend/gen" \
  --cs-out "$ROOT/rampage-client/Assets/Rampage/Common/Scripts/WorldNetwork/Generated/rampage"
```

This discovers rampage-backend's commands (the shared world-engine lobby plugin is owned by another module, so it's skipped here and generated separately), writes the Go proto package to `rampage-backend/gen`, and writes the C# into `WorldNetwork/Generated/rampage`, where protoc lays each package out in its own namespace-mirrored subdirectory (so it sits beside the separately-generated `Generated/lobby`). The generated wire methods live in each type's own package, so they compile in wherever the type is already used — no blank import or registration step.

**Generate just the client SDK from a published backend** (no backend checkout; pin to a release):

```shell
world sdk generate github.com/Argus-Labs/rampage-backend \
  --ref v1.4.0 \
  --cs-out "$ROOT/rampage-client/Assets/Rampage/Common/Scripts/WorldNetwork/Generated/rampage"
```

### Cardinal Game Engine

- **`world start`** — Start Cardinal game shard in production mode
- **`world stop`** — Stop running Cardinal services
- **`world purge`** — Remove all Cardinal containers and data

### Cloud Deployment

- **`world deploy`** — Deploy your project to World Engine cloud
- **`world status`** — Check deployment status and health
- **`world logs`** — View real-time application logs
- **`world destroy`** — Remove cloud deployment

### Organization & Project Management

- **`world organization create`** — Create a new organization
- **`world organization members`** — Show members in a organization
- **`world project create`** — Create a new project
- **`world project update`** — Update project configuration
- **`world project delete`** — Delete a project

### User Management

- **`world user invite`** — Invite users to your organization
- **`world user role`** — Change user roles in organization
- **`world user update`** — Update your user profile

### Getting Help

- **`world help`** — Show general help information
- **`world <command> --help`** — Show help for specific commands

<br/>

## How It Works

### Architecture

World CLI is built around several key components:

1. **Cardinal Engine** — The core game engine for World Engine projects
2. **Cloud Infrastructure** — World Engine's managed cloud platform
3. **Project Templates** — Pre-built starter templates for different game types
4. **Docker Integration** — Containerized development and deployment

### Development Workflow

1. **Setup** → Initialize a new project with `world setup`
2. **Develop** → Use `world cardinal start/stop` for local development
3. **Deploy** → Use `world deploy` to push to cloud
4. **Monitor** → Use `world status` and `world logs` for monitoring

### Cloud Deployment

World CLI manages the complete deployment lifecycle:

- **Infrastructure Provisioning** — Automatically sets up required cloud resources
- **Container Orchestration** — Manages Docker containers and networking
- **Environment Management** — Handles dev/staging/production environments
- **Monitoring & Logging** — Provides real-time status and log access

### Security & Authentication

- **OAuth Integration** — Secure authentication via World Engine accounts
- **Role-Based Access** — Granular permissions for teams and organizations
- **Environment Isolation** — Separate configurations for different environments

<br/>

## Troubleshooting

### Common Issues

**"Command not found"**
- Ensure the binary is in your PATH (`/usr/local/bin/` for Linux/macOS, `$env:USERPROFILE\go\bin` for Windows)
- Try downloading the latest release from [GitHub Releases](https://github.com/Argus-Labs/world-cli/releases)

**Authentication errors**
- Run `world login` to re-authenticate
- Check your internet connection

**Docker issues**
- Ensure Docker is running
- Run `world doctor` to check Docker setup

**Cardinal won't start**
- Check if ports 4222 (NATS) and 8080 (Gateway) are available
- Run `world purge` to clean up containers

### Support

- **Documentation** — [World Engine Docs](https://world.dev)
- **Community** — [Telegram Chat](https://t.me/worldengine_dev)
- **Issues** — [GitHub Issues](https://github.com/Argus-Labs/world-cli/issues)

<br/>
