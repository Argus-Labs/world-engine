<div align="center">
  <img src="https://i.imgur.com/P6YpZCT.png" width=250 />
  <br/>
  The world’s first Gamechain SDK that utilizes Argus Labs’ novel sharded rollup architecture.
  <br/>
  <br/>
  <p align="center">
    <a href="https://codecov.io/gh/Argus-Labs/world-engine" >
    <img src="https://codecov.io/gh/Argus-Labs/world-engine/branch/main/graph/badge.svg?token=XMH4P082HZ"/>
    </a>
    <a href="https://goreportcard.com/report/pkg.world.dev/world-engine/cardinal"><img src="https://goreportcard.com/badge/pkg.world.dev/world-engine/cardinal" alt="Go Report Card">
    </a>
    <a href="https://t.me/worldengine_dev" target="_blank">
    <img alt="Telegram Chat" src="https://img.shields.io/endpoint?color=neon&logo=telegram&label=chat&url=https%3A%2F%2Ftg.sumanjay.workers.dev%2Fworldengine_dev">
    </a>
    <a href="https://pkg.go.dev/pkg.world.dev/world-engine/cardinal" target="_blank">
    <img src="https://pkg.go.dev/badge/pkg.world.dev/world-engine/cardinal.svg" alt="Go Reference">
    </a>
    <a href="https://x.com/WorldEngineGG" target="_blank">
    <img alt="Twitter Follow" src="https://img.shields.io/twitter/follow/WorldEngineGG">
    </a>
  </p>
</div>

## Overview

World Engine allows onchain games to scale to thousands of transactions per second with sub-100ms block time, while
increasing development speed significantly. Sharding enables game developers to distribute their game load across
various shards.

## Getting Started

The simplest way to get started with World Engine is to follow the World
Engine [quickstart guide](https://world.dev/quickstart)

Note, this repo is for the core development of the World Engine only, and should not be used for developing World Engine
powered games.

## Documentation

For an in-depth guide on how to use World Engine, visit our [documentation](https://world.dev).

## Directory Structure

<pre>
◢ ✦ ◣ World Engine ◢ ✦ ◣
├── <a href="./assert">assert</a>: Custom testing package that includes stack traces in errors.
├── <a href="./cardinal">cardinal</a>: The first World Engine game shard implementation.
├── <a href="./evm">evm</a>: Rollkit and Polaris integrated Base Shard rollup.
├── <a href="./relay">relay</a>: Game Shard message relayer. Currently contains one implementation using Nakama.
├── <a href="./rift">rift</a>: Protobuf definitions and generated Go code for the World Engine's cross shard messaging protocol.
├── <a href="./sign">sign</a>: Library to facilitate message signing and verification.
</pre>
