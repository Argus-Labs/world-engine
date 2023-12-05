<h1 align="center"> World Engine ◢ ✦ ◣ </h1>

![https://i.imgur.com/iNFuo81.jpeg](https://i.imgur.com/iNFuo81.jpeg)

<div>
  <a href="https://codecov.io/gh/Argus-Labs/world-engine" >
    <img src="https://codecov.io/gh/Argus-Labs/world-engine/branch/main/graph/badge.svg?token=XMH4P082HZ"/>
  </a>
</div>

World Engine is a sharded layer 2 blockchain SDK designed with the needs of game developers and players in mind. The
World Engine’s main innovation lies in its sharding design inspired by the server architecture of computationally
intensive massively multiplayer online (MMO) games.

Sharding enables game developers to distribute their game load across various shards. Consequently, a World Engine chain
can adjust its throughput in response to demand, growing in tandem with the developer or publisher. At the same time,
World Engine’s sharding architecture also avoids the interoperability/platform fragmentation issues associated with
scaling by spinning up another separate rollup.

## Getting Started

The simplest way to get started with World Engine is to build a game shard using Cardinal. Note, this repo is for the core development of the World Engine only, and should not be used for developing World Engine powered games.

Using the world-cli, you can get started with your own Cardinal game shard. Please see [getting started](https://world.dev/Cardinal/getting-started) for more details.


## Documentation

To learn how to build your own World Engine powered game, visit our [documentation](http://world.dev).

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

## 🚧 WARNING: UNDER CONSTRUCTION 🚧

This project is work in progress and subject to frequent changes as we are still working on wiring up the final system.
It has not been audited for security purposes and should not be used in production yet.