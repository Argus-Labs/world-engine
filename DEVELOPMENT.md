<div align="center"> <!-- markdownlint-disable-line first-line-heading -->
  <img alt="World Engine" src="https://i.imgur.com/P6YpZCT.png" width=250 />
  <br/>
  The world’s first Gamechain SDK that utilizes Argus Labs’ novel sharded rollup architecture.
  <br/>
  <br/>
  <a href="https://codecov.io/gh/Argus-Labs/world-engine" >
    <img alt="Code Coverage" src="https://codecov.io/gh/Argus-Labs/world-engine/branch/main/graph/badge.svg?token=XMH4P082HZ"/>
  </a>
  <a href="https://goreportcard.com/report/pkg.world.dev/world-engine/cardinal">
    <img src="https://goreportcard.com/badge/pkg.world.dev/world-engine/cardinal" alt="Go Report Card">
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
</div>

## Info for World Engine Developers

If you are looking for info for developing games using the World Engine, try:

- the [README.md](./README.md)
- the World Engine [quickstart guide](https://world.dev/quickstart)
- or the online [documentation](https://world.dev).

## Dev Tools

Internal development is done using Jet Brains GoLand

## Directory Structure

<pre>
◢ ✦ ◣ World Engine ◢ ✦ ◣
├── <a href="./.run">.run</a>: Configurations for Running and Debugging in GoLand IDE
├── <a href="./assert">assert</a>: Custom testing package that includes stack traces in errors.
├── <a href="./cardinal">cardinal</a>: The first World Engine game shard implementation.
├── <a href="./e2e">e2e</a>: Test Games for End-to-End testing.
├── <a href="./evm">evm</a>: Rollkit and Polaris integrated Base Shard rollup.
├── <a href="./relay">relay</a>: Game Shard message relayer. Currently contains one implementation using Nakama.
├── <a href="./rift">rift</a>: Protobuf definitions and generated Go code for the World Engine's cross shard messaging protocol.
├── <a href="./scripts">script</a>: Scripts used for development.
├── <a href="./sign">sign</a>: Library to facilitate message signing and verification.
</pre>

## Running Tests from GoLand

From the Configurations menu at the top right of the GoLand window, choose `World Engine Docker - Test Game` and run it. You will see:

<pre>
/usr/local/bin/docker compose -f ./world-engine/docker-compose.yml -p world-engine up --no-deps cockroachdb nakama redis game-debug
[+] Running 6/5
✔ Network world-engine_world-engine  Created                                                                                                                                                                    0.0s
✔ Volume "world-engine_data"         Created                                                                                                                                                                    0.0s
✔ Container cockroachdb              Created                                                                                                                                                                    0.0s
✔ Container redis                    Created                                                                                                                                                                    0.0s
✔ Container test_game                Created                                                                                                                                                                    0.0s
✔ Container relay_nakama             Created                                                                                                                                                                    0.0s
✔ Container test_nakama              Created                                                                                                                                                                    0.0s
Attaching to cockroachdb, redis, relay_nakama, test_game, test_nakama
</pre>

After a short time, you will see `test_game` ticking, along with messages from `relay_nakama` and `test_nakama`:

<pre>
test_game  | 7:19PM INF Tick completed duration=1.02933ms tick=1385 tx_count=0
</pre>

If you check the logs from the `test_nakama` container ( cmd-8 for Services, then click on
Docker > Docker-compose-world-engine > test_nakama > test_nakama). You should see test result like this:

<pre>
=== RUN   TestEvents
2024-10-09T19:27:34.724721412Z --- PASS: TestEvents (1.26s)
2024-10-09T19:27:34.724799246Z === RUN   TestReceipts
2024-10-09T19:27:36.722025883Z --- PASS: TestReceipts (2.00s)
2024-10-09T19:27:36.722052799Z === RUN   TestTransactionAndCQLAndRead
2024-10-09T19:27:38.719295228Z --- PASS: TestTransactionAndCQLAndRead (2.00s)
2024-10-09T19:27:38.719301812Z === RUN   TestCanShowPersona
2024-10-09T19:27:39.738128589Z --- PASS: TestCanShowPersona (1.02s)
2024-10-09T19:27:39.738162881Z === RUN   TestDifferentUsersCannotClaimSamePersonaTag
2024-10-09T19:27:39.795977744Z --- PASS: TestDifferentUsersCannotClaimSamePersonaTag (0.06s)
2024-10-09T19:27:39.795986911Z === RUN   TestConcurrentlyClaimSamePersonaTag
2024-10-09T19:27:39.898364637Z --- PASS: TestConcurrentlyClaimSamePersonaTag (0.10s)
2024-10-09T19:27:39.898377304Z === RUN   TestCannotClaimAdditionalPersonATag
2024-10-09T19:27:40.736493221Z --- PASS: TestCannotClaimAdditionalPersonATag (0.84s)
2024-10-09T19:27:40.736809012Z === RUN   TestPersonaTagFieldCannotBeEmpty
2024-10-09T19:27:40.752374922Z --- PASS: TestPersonaTagFieldCannotBeEmpty (0.02s)
2024-10-09T19:27:40.752464213Z === RUN   TestPersonaTagsShouldBeCaseInsensitive
2024-10-09T19:27:41.734335605Z --- PASS: TestPersonaTagsShouldBeCaseInsensitive (0.98s)
2024-10-09T19:27:41.734346729Z === RUN   TestReceiptsCanContainErrors
2024-10-09T19:27:43.719725487Z --- PASS: TestReceiptsCanContainErrors (1.98s)
2024-10-09T19:27:43.719743778Z === RUN   TestInvalidPersonaTagsAreRejected
2024-10-09T19:27:45.719720074Z --- PASS: TestInvalidPersonaTagsAreRejected (2.00s)
2024-10-09T19:27:45.719746240Z === RUN   TestAuthenticateSIWE
2024-10-09T19:27:45.772216412Z --- PASS: TestAuthenticateSIWE (0.05s)
2024-10-09T19:27:45.772261787Z PASS
2024-10-09T19:27:45.774864654Z ok  github.com/argus-labs/world-engine/e2e/tests/nakama  12.328s</pre>

## Running Tests in the Debugger from GoLand

From the Configurations menu at the top right of the GoLand window, choose `World Engine Docker - Test Game Debug`
and run it (make sure to stop the `world-engine-game` and `world-engine-nakama` containers first if they were already
running. You will see:

<pre>
/usr/local/bin/docker compose -f ./world-engine/docker-compose.yml -p world-engine up --no-deps cockroachdb nakama redis game-debug
[+] Running 6/5
✔ Network world-engine_world-engine  Created                                                                                                                                                                    0.0s
✔ Volume "world-engine_data"         Created                                                                                                                                                                    0.0s
✔ Container cockroachdb              Created                                                                                                                                                                    0.0s
✔ Container redis                    Created                                                                                                                                                                    0.0s
✔ Container test_game-debug          Created                                                                                                                                                                    0.0s
✔ Container relay_nakama             Created                                                                                                                                                                    0.0s
Attaching to cockroachdb, redis, relay_nakama, test_game-debug
[...]
test_game-debug  | API server listening at: [::]:40000
test_game-debug  | 2024-10-09T19:36:11Z warning layer=rpc Listening for remote connections (connections are not authenticated nor encrypted)
</pre>

Those lines near the top of the logs about API server and listening for remote connections show the debugger is ready.
There will also be a lot of warnings about `relay_nakama` failing to establish websocket connection. Those show attempts
by Nakama to attach to Cardinal, but Cardinal is waiting for the remote debugging session to start, so they will continue
until you complete the next step.

Now use the Configurations menu again to choose `Cardinal Debug`. Before you hit the debug icon beside it, try setting
a breakpoint in the `main()` function in `e2e/testgames/game/main.go`. Now hit the debug icon. You should hit that
breakpoint, and from there be able to use the debugger normally including stepping into World Engine code.

Unfortunately, you will NOT be able to debug the `relay` code, because that runs in the nakama container.

If you hit continue from that breakpoint, you can restart the `test_nakama` container with

```shell
docker compose test_nakama up
```

and watch those tests run again. You should get the same output.

## Submitting Changes

Before you push changes, including creating or updating a pull request, please be sure you have done the following
steps to be sure that your PR will pass the CI tests. These command are run from the `world-engine` directory.

```shell
make lint
make unit-test-all
make e2e-nakama
make e2e-evm
```

If any of those fail, you can be sure your push will fail to pass the CI tests, and you should fix those issues before
pushing to origin.
