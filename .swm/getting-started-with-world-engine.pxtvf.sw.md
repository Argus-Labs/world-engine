---
id: pxtvf
title: Getting Started with World Engine
file_version: 1.1.2
app_version: 1.7.1
---

## Components

The World Engine comprises of two main components:

*   Cardinal

    *   Game Server designed to run on-chain games.

    *   A Go server with a custom ECS implementaiton, Redis, and a blockchain connector.

*   Cosmos Rollkit Rollup

    *   A rollup blockchain that handles assets, EVM scripting, accounts, etc.

*   Nakama

    *   Server that handles client connections and data relay to Cardinal.

These components communicate over a secure gRPC channel to share data and execute transactions.

## Running the Stack (Docker)

The World Engine comes with a few preconfigured `Dockerfile`s to quickly get the entire system running in Docker. There are three `Dockerfiles` needed to run the system:

*   Celestia - the DA layer

*   `📄 .archive.chain/rollup.Dockerfile` - the rollup

*   `📄 game/nakama/Dockerfile` - the preconfigured relay server

*   `📄 cardinal/Dockerfile` - Game server/ECS

All components can be ran with a simple script inside the `📄 .archive.chain/Makefile`.

Enter the following command in the `📄 chain` directory to start the services:

```
make start-services
```

⚠️NOTE⚠️ You will see some errors in the rollup container while the Celestia DA node boots up. This will occur for a few seconds until the rollup connects to the DA layer.

<br/>

<br/>

`📄 .archive.chain/Makefile` command that runs all required services in a single docker container.
<!-- NOTE-swimm-snippet: the lines below link your snippet to Swimm -->
### 📄 .archive.chain/Makefile
```chain/makefile
330    start-services:
331    	@echo "Starting services"
332    	$(shell ../game/nakama/setup.sh)
333    	@docker-compose down -v --remove-orphans
334    	@docker-compose build
335    	@docker-compose up --abort-on-container-exit --exit-code-from postgres nakama celestia node
```

<br/>

## Interacting with the Services

Interacting with the rollup is easiest via gRPC. The `📄 .archive.chain/rollup.Dockerfile` exposes the gRPC port to the localhost on port `9090`. The simplest way to interact with the rollup is by building the binary and using the World Engine CLI. This makes it easy to query the blockchain and send transactions. The rollup binary can be built from the following command in the `📄 .archive.chain/Makefile`:

<br/>

<br/>

Simply enter `make` `build`<swm-token data-swm-token=":.archive.chain/Makefile:142:0:0:`build: BUILD_ARGS=-o $(BUILDDIR)/`"/> from the root of the project. This will create a binary in the `build` directory called `argusd`.
<!-- NOTE-swimm-snippet: the lines below link your snippet to Swimm -->
### 📄 .archive.chain/Makefile
```chain/makefile
142    build: BUILD_ARGS=-o $(BUILDDIR)/
```

<br/>

Interacting with the preconfigured Nakama ECS Game server can be done either over gRPC or using the web interface. Follow the instructions below to access the web interface:

<br/>

<br/>

Instructions to access the Nakama Web Interface.
<!-- NOTE-swimm-snippet: the lines below link your snippet to Swimm -->
### 📄 game/nakama/readme.md
```markdown
1      go to localhost:7351 after container finishes initialization
2      
3      enter in credentials:
4      
5      username: admin
6      password: password
7      ref: https://youtu.be/Ru3RZ6LkJEk
```

<br/>

The Nakama server can also be interacted with via Nakama client libraries. See the following example below on how to interact with Nakama via the C# client library. The code below shows how to establish a connection, as well as call a custom RPC endpoint.

```csharp
public class NakamaConn : MonoBehaviour
{

    private string scheme = "http";
    private string host = "localhost";
    private int port = 7350;
    private string serverKey = "defaultkey";

    private IClient client;
    private ISession sesh;

    private ISocket sock;

    // Start is called before the first frame update
    async void Start()
    {
        // Establish Connection
        client = new Client(scheme, host, port, serverKey, UnityWebRequestAdapter.Instance);
        sesh = await client.AuthenticateDeviceAsync(SystemInfo.deviceUniqueIdentifier);
        sock = client.NewSocket();
        await sock.ConnectAsync(sesh, true);

        // Call custom RPC endpoint
        var res = await sock.RpcAsync("mint-coins");
        Debug.Log(res);
    }
}
```

## Architecture Diagram

<br/>

<!--MERMAID {width:100}-->
```mermaid
flowchart LR
2("Cosmos Rollup") --- 3("Celestia DA")

4("Cardinal") --- |"gRPC connection"|1("Nakama")
4("Cardinal") ---> |"gRPC connection"|2("Cosmos Rollup")
2("Cosmos Rollup") ---> |"gRPC connection"|4("Cardinal")

5("game client") ---> |"JSON RPC/HTTPs"| 1("Nakama")
```
<!--MCONTENT {content: "flowchart LR<br/>\n2(\"Cosmos Rollup\") --- 3(\"Celestia DA\")\n\n4(\"Cardinal\") --- |\"gRPC connection\"|1(\"Nakama\")<br/>\n4(\"Cardinal\") -\\-\\-\\> |\"gRPC connection\"|2(\"Cosmos Rollup\")<br/>\n2(\"Cosmos Rollup\") -\\-\\-\\> |\"gRPC connection\"|4(\"Cardinal\")\n\n5(\"game client\") -\\-\\-\\> |\"JSON RPC/HTTPs\"| 1(\"Nakama\")"} --->

<br/>

<br/>

<br/>

This file was generated by Swimm. [Click here to view it in the app](https://app.swimm.io/repos/Z2l0aHViJTNBJTNBd29ybGQtZW5naW5lJTNBJTNBQXJndXMtTGFicw==/docs/pxtvf).
