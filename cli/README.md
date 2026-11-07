<div align="center"> <!-- markdownlint-disable-line first-line-heading -->
<img alt="World CLI Logo" src="https://i.imgur.com/XM74ODi.png" width="378">
<p>A swiss army knife for creating, managing, and deploying World Engine projects</p>
  <p>
    <a href="https://codecov.io/gh/Argus-Labs/world-engine" >
    <img alt="Codecov" src="https://codecov.io/gh/Argus-Labs/world-engine/branch/main/graph/badge.svg"/>
    </a>
    <a href="https://goreportcard.com/report/github.com/argus-labs/world-engine/cli">
    <img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/argus-labs/world-engine/cli">
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

World CLI is a [Go tool](https://go.dev/doc/modules/managing-dependencies#tools). You need [Go](https://go.dev/doc/install) 1.27.1 or later.

**Windows Users:** Windows Subsystem for Linux 2 (WSL2) is required for running World CLI on Windows.

Install the `world` command, and make sure `$(go env GOPATH)/bin` is on your `PATH`:

```shell
go install github.com/argus-labs/world-engine/cli/cmd/world@latest
world setup
```

The CLI ships in the `github.com/argus-labs/world-engine` module, so each project pins its CLI through its World Engine version. Inside a project that declares the tool, `world` runs the project's version (`go tool world`), whichever version is installed globally. Projects created with `world setup` already declare it. To add it to an existing project at the project's World Engine version:

```shell
go mod edit -tool=github.com/argus-labs/world-engine/cli/cmd/world
go mod tidy
```

Upgrading World Engine (`go get github.com/argus-labs/world-engine@latest`) upgrades the project's CLI with it. Rerun `go install` to update the global `world`.

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

The new project also gets an agent skill at `.claude/skills/world-local-dev/SKILL.md` (and `.agents/skills/`) that teaches Claude Code, Codex and similar tools how to run, reload, inspect and reset the game with `world`.

### 3. Run your project

Start your project:

```shell
world start
```

This builds your shard images and runs them as Docker containers, with NATS and Postgres alongside. Shards are reachable at `http://localhost:8080/<project>/<instance>` through a proxy inside `world start`; Ctrl+C stops the containers and keeps their data.

Shards trust the dev identity header by default. To sign in with real Argus Auth accounts, as hosted environments do, add to `world.toml`:

```toml
[auth]
mode = "argus"
url = "https://api.argus.dev"
```

<br/>

## Available Commands

- **`world setup`** — Initialize a new World Engine project from templates
- **`world doctor`** — Check that all required dependencies are installed
- **`world version`** — Display version information and check for updates

### Cardinal Game Engine

- **`world start`** — Build the shards and run the world on Docker (stays attached; Ctrl+C stops it)
- **`world reload`** — Rebuild and recreate the shard containers (`--purge` wipes their state first)
- **`world stop`** — Stop the containers; database and NATS state are kept
- **`world purge`** — Remove the project's containers, volumes and network (`--image` also prunes the built images)

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

**"command not found: world" or "no such tool"**
- Install `world` with `go install github.com/argus-labs/world-engine/cli/cmd/world@latest` and put `$(go env GOPATH)/bin` on your `PATH`
- `go tool world` works only inside a project whose `go.mod` declares the tool. Add it with `go mod edit -tool=github.com/argus-labs/world-engine/cli/cmd/world && go mod tidy`

**Authentication errors**
- Run `world login` to re-authenticate
- Check your internet connection

**Docker issues**
- Ensure Docker is running
- Run `world doctor` to check Docker setup

**Cardinal won't start**
- Run `world doctor`: it checks that the Docker daemon is reachable
- Check that ports 8080 (shard API), 4222 (NATS) and 5432 (project database) are free on your machine
- Upgrading from a CLI that used k3d: its cluster may still hold those ports — run `k3d cluster delete world-engine` once
- Run `world purge` to start from clean containers and volumes

### Support

- **Documentation** — [World Engine Docs](https://world.dev)
- **Community** — [Telegram Chat](https://t.me/worldengine_dev)
- **Issues** — [GitHub Issues](https://github.com/Argus-Labs/world-engine/issues)

<br/>
