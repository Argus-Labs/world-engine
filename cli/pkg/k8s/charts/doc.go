// Package charts holds the Helm charts world-engine publishes (ADR-066). They are
// not embedded in the CLI: local worlds run on Docker (cli/pkg/local), and the
// cli/pkg/local contract test renders cardinal-shard to keep both in step.
package charts
