# Charts

Helm charts world-engine publishes on each release (ADR-066). They render shards on any Kubernetes cluster; `world start` runs the same shards as Docker containers and does not use them.

| Chart | Renders | Who uses it |
|---|---|---|
| `cardinal-shard` | one Deployment + Service + PDB per pool instance; routes when `routing.provider` is `gke` | monorepo (synced copy), ephemeralgen, `helm install` |
| `nats` | the official `nats/nats` chart with a one-node JetStream preset | `helm install` |
| `postgres` | one Postgres 16 with a PVC and a ClusterIP, NodePort or LoadBalancer Service | `helm install` |

Values are the deployment contract. `values.schema.json` lists every key and rejects unknown ones.

```sh
helm show values oci://ghcr.io/argus-labs/charts/cardinal-shard --version <world-engine version without the v, e.g. 1.0.1>
helm install gameplay oci://ghcr.io/argus-labs/charts/cardinal-shard --version <v> -f my-values.yaml
```

Minimum values: `shardID`, `image`, `imageTag`, `poolSize`, `organization`, `project`, `region`, `nats.url`, `auth.mode`. See `cardinal-shard/examples/`.

`go test ./cli/pkg/k8s/charts/` renders every example and checks the schema; `cli/pkg/local` has a contract test that keeps the Docker environment `world start` builds equal to what `examples/local.yaml` renders. The monorepo copy is kept in sync by CI (ADR-066 step 2).
