# Rampage Backend

## Prerequisites

- [Go 1.24 or later](https://go.dev/doc/install)
- Docker and Kubernetes (for running with Tilt)
  - [Orbstack](https://orbstack.dev/download) (for macOS)
  - [Docker Desktop](https://www.docker.com/products/docker-desktop/) (for Windows)
- [Ko](https://ko.build/install/)
- [Task](https://taskfile.dev/docs/installation)
- If you run into errors, install these too:
  - [Helm](https://helm.sh/docs/intro/install/)
    - Then run `helm repo add nats https://nats-io.github.io/k8s/helm/charts/`
  - [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/)
    - Then run `kind create cluster --name tilt-cluster`

## Running the Entire Development Stack

Start the development environment using Task:

```bash
task dev:rampage

# then visit http://localhost:10350 (Tilt UI) to check the status of each service.
```

When successful, http://localhost:10350 should roughly look like this:

![](./readme-images/tilt-success.png)

## Registering Persona

During development, sending commands require Persona ID, which you can get from running:

```bash
# in the root of the monorepo
go run pkg/cardinal/examples/basic/cmd/client/main.go register-persona

# example of a successful registration:
# Successfully registered and stored persona: 86aa9b07b619664bc16c3b987f46b805467c4852e413903d4ecf1919cca0eed6
```

Copy the **persona** string that you get from running the above script, and paste it onto Unity.

## World Engine and Cardinal Docs

> Since you're still developing here on the monorepo (which contains all the source code for World Engine v2), you can skip the entirety of Quickstart docs that you might find below. Simply doing `task dev:rampage` is enough, there's no need to install World CLI nor do `world create`

To familiarize yourself with development using World Engine v2, you may refer to [introductory World Engine v2 docs](https://argus-cd64690a.mintlify.app/introduction),
followed by [Cardinal docs](https://argus-cd64690a.mintlify.app/cardinal/introduction).

## Resetting Data

Due to hot reloads and such, sometimes development data might become corrupt. In that case, you can reset the **data** using:

```bash
tilt down rampage

# then rerun the stack using task dev:rampage
```

## Argus Auth (formerly Argus ID)

Don't worry about it for now, you'll be developing with _auth disabled_ (aka dev auth) until further notice.

> The section below is still a stub and not guaranteed to work.

If you're really curious, you can run with auth enabled:

1. `task dev:argus-auth` (this will run Argus Auth on your http://localhost:3000 and http://localhost:3001 for backend and frontend respectively)
2. In `k8s/gateway/deployment.yaml`:
   - Change `DISABLE_AUTH` value to `"false"`
   - Add `ARGUS_ID_URL` with value `"host.docker.internal:3000"`
3. Remove `AuthInterceptor = new DevAuthInterceptor(...)` from Unity code
