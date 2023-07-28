# Sample Game Server Shard

This is a sample game server shard built using Cardinal and [Nakama](https://heroiclabs.com/nakama/) as the network
relayer.

For a more detailed readme,
see [Getting Started with Cardinal](https://coda.io/d/Getting-Started-with-Cardinal_dvcS4DQePrC/Getting-Started-with-Cardinal_su6d-#_luof7).

# Prerequisites

## Mage Check

A mage target exists that will check for some common preqrequisites. Run the check with:

```bash
mage check
```

## Github Access

The gameplay server (under the `server` directory) makes use of Cardinal, which is a library in the
private [world-engine](https://github.com/Argus-Labs/world-engine) repo.

You likely have access to this repo, but the `go` binary sometimes has trouble accessing private repos on your behalf.

### GOPATH

[Configure Go to access private modules](https://www.digitalocean.com/community/tutorials/how-to-use-a-private-go-module-in-your-own-project#configuring-go-to-access-private-modules)

TL;DR: Add 'GOPRIVATE="github.com/argus-labs/world-engine"' to your environment variables.

### Github Credentials

In addition, configure git to use your private credentials via HTTPS or SSH:

[Providing Private Module Credentials for HTTPS](https://www.digitalocean.com/community/tutorials/how-to-use-a-private-go-module-in-your-own-project#providing-private-module-credentials-for-https)
OR
[Providing Private Module Credentials for SSH](https://www.digitalocean.com/community/tutorials/how-to-use-a-private-go-module-in-your-own-project#providing-private-module-credentials-for-ssh)

TODO: It would be helpful if this section included the error message that a user could expect to see when their git
credentials are incorrect.

## Docker Compose

Docker and docker compose are required for running Nakama and both can be installed with Docker Desktop.

[Installation instructions for Docker Desktop](https://docs.docker.com/compose/install/#scenario-one-install-docker-desktop)

## Mage

[Mage](https://magefile.org/) is a cross-platform Make-like build tool.

```bash
git clone https://github.com/magefile/mage
cd mage
go run bootstrap.go
```

# Running the Server

To start nakama and the gameplay server:

```bash
mage start
```

To restart JUST the gameplay server:

```bash
mage restart
```

To stop Nakama and gameplay servers:

```bash
mage stop
```

Alternatively, killing the `mage start` process will also stop Nakama and the gameplay server.

Note: if any server endpoints have been added or removed Nakama must be relaunched (via `mage stop` and `mage start`).

To start JUST the Nakama server
```bash
mage nakama
```
Note: Nakama depends on the gameplay server, so you'll have to start that separately. This command is useful when you want to start the debug server in debug mode to step through code and troubleshoot.

# Verify the Server is Running

Visit `localhost:7351` in a web browser to access Nakama. For local development, use `admin:password` as your login
credentials.

The Account tab on the left will give you access to a valid account ID.

The API Explorer tab on the left will allow you to make requests to the gameplay server.

# Claim PersonaTags and sign transactions

By default, Nakama will sign transactions with a private key. Before transactions can be signed, a PersonaTag must be associated with the Nakama UserID. This is done by:

- Visit localhost:7351 and log in with "admin:password"
- On the left banner, click the "API Explorer" tab
- Create a new Nakama user:
  - Select the "AuthenticateDevice" endpoint in the dropdown and set the request body to:
  - {"account": {"id": "1234567890"}}, "create": true, "username": "some-user-name"}
  - Hit Send Request
- On the left banner, click the "Accounts" tab
- Find the newly created user and copy the User ID
- Go back to "API Explorer" and paste the User ID into the "set user ID as request context" box.
- Select the "namaka/claim_persona" endpoint in the dropdown.
- Set the Request Body to {"PersonaTag": "<some-persona-tag>"}
- Hit Send Request
- Verify the UserID to PersonaTag association was successful:
- Select the "nakama/show_persona" endpoint in the dropdown
- Hit Send Request. You should see "Status": "accepted" in the response
- That's it. Future transactions will automatically be signed with this PersonaTag and the Nakama private key.

# Copy the Sample Game Server

You can make a full copy of nakama, the cardinal server, and the mage build targets with:

```bash
mage copy <target-directory> <module-path>
```

The <target-directory> is where you want your code to live on your local machine and the <module-path> parameter is the
repo location of your new game.

See the [go mod init](https://golang.org/ref/mod#go-mod-init) documentation for more details about the module path
parameter.
