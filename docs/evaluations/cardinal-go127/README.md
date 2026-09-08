# Cardinal generic methods evaluation

For common gameplay operations, prioritize checked `Get`, `Set`, `Has`, and `Remove` methods on `BaseSystemState` for known entity IDs. Keep typed references inside query loops. See the [focused public API evaluation](PUBLIC_API.md) for concrete calls and gameplay benchmarks.

The query projection and test-helper analysis below covers additional opportunities. The measurements do not support a performance claim for generic method syntax itself.

The repository now pins Go 1.27.1 in `go.mod`, `.prototools`, and all four CI toolchain declarations. The pull advanced the base to `d663f5b0`. The linter pin is updated to 2.13.2 because 2.6.2 cannot read Go 1.27 export data, even when built with Go 1.27.1. Production APIs are unchanged. The `.go.txt` files are throwaway prototypes compiled inside an isolated copy of Cardinal.

## Additional API opportunities

### Add projection to the existing query iterator

Current code must project inside a loop or pass the iterator to a package-level function. The new method introduces an output type while preserving the entity ID.

```diff
- for id, player := range state.Players.Iter() {
-     summary := summarize(id, player)
+ for id, summary := range state.Players.Iter().Map(summarize) {
      consume(id, summary)
  }
```

The prototype compiles this signature and checks chaining with `Filter` and `Single`.

```go
func (s SearchResult[E, C]) Map[U any](
	project func(E, C) U,
) SearchResult[E, U]
```

`SearchResult` already owns `Filter`, `Limit`, and `Single` in [system.go](../../../pkg/cardinal/system.go). `Map` adds a type-changing operation at the same boundary. Type inference derives `U` from the callback. Evaluation remains lazy, and breaking iteration stops upstream work. Mapping preserves entity IDs. A nil callback would panic when consumed, as an ordinary call would.

**Recommendation.** Add `Map` for read models and query composition. Keep direct loops available for tick paths where these microseconds matter. Do not materialize a slice unless the caller needs one. A package-level generic function is a viable alternative with the same measured cost, but it makes chained calls harder to read.

### Move fixture functions onto their existing owners

The following current calls belong to unpublished local `TestWorld` work, not the pulled main branch. The evaluation preserves those files. Once that work builds, direct generic methods are the smallest improvement.

```diff
- hp := cardinal.Get[Health](h, id)
- cardinal.Set(h, id, Health{HP: 100})
- deaths := cardinal.Events[Death](tick)
+ hp := h.Get[Health](id)
+ h.Set(id, Health{HP: 100})
+ deaths := tick.Events[Death]()
```

The same change fits `Has`, `Remove`, `Entities`, `Single`, `SystemEvents`, and `ShardCommands`. Keep fixture timing checks, test failure reporting, consistency checks, and event filtering inside their current owners. Method syntax does not remove heterogeneous event storage or its type assertions. These helper migrations are design sketches, not compiled or benchmarked implementations.

Two designs are possible:

| Design | Caller | Benefit | Cost |
| --- | --- | --- | --- |
| Direct methods, recommended | `h.Get[Health](id)` | Existing owner, easy autocomplete | Generic methods cannot satisfy interface methods |
| Typed capability | `health := h.Components[Health](); health.Get(id)` | Ordinary instantiated methods can satisfy narrow interfaces | Another type and handle to learn |

Use a typed capability only when callers need an interface or repeatedly pass access to one component type. Do not cache column pointers across archetype changes or world resets.

### Keep declared system dependencies

Current systems declare `Contains[Row]`, `WithCommand[T]`, and typed event fields. Their initialization registers components, typed queues, debug schemas, and service handlers before ticks. See `initSystemFields`, `WithCommand.init`, and `WithEvent.init` in [system.go](../../../pkg/cardinal/system.go).

The earlier assertion-based entity prototype made ad hoc reads look shorter. The [focused gameplay proposal](PUBLIC_API.md) supersedes that error contract with checked Get and Set for arbitrary IDs:

```diff
- hp := player.Health.Get()
- player.Health.Set(Health{HP: hp.HP - 1})
+ hp := entity.Get[Health]()
+ entity.Set(Health{HP: hp.HP - 1})
```

The compiled prototype compares the equivalent component access mechanics. Both variants call the existing ECS Get and Set functions with the same checks. It does not implement a new registration or query model.

**Recommendation.** Keep typed query rows as the default. A generic entity handle allows calls for components outside the declared row and moves missing-component failures toward runtime. Calls such as `state.Commands[T]()` also need an explicit registration design. Generic syntax alone cannot replace the initialization contract.

The latest `RegisterSystemV2[S System]` accepts a caller-owned struct with `Run()`. Its receiver-method replacement can be an ordinary `world.RegisterSystem(s System, opts...)`. A generic parameter adds no capability there. The older function registration can use `world.RegisterSystemFunc[T](fn func(*T), opts...)`. Distinct names avoid an overload that Go does not provide.

## Measurements

Measured on 2026-09-07 on one Apple M5 Max, macOS arm64. Ten samples per case, 200 ms per sample, `GOMAXPROCS=1`, one benchmark CPU. Toolchain order alternates between paired samples. API order within a process is fixed. No test builds or other task benchmarks ran during the measured sampling window. Other host activity was not controlled.

All variants operate on the same real Cardinal ECS implementation and fixtures. Setup and validation are outside the timer. Projection scans read Position and Velocity, construct one small value, and sum both coordinates. Checksums are validated after every measured invocation. Component access repeatedly reads, increments, and writes one resident entity. Assertions remain enabled. These are steady-state microbenchmarks, not tick, network, snapshot, or entity-churn benchmarks.

### API cost on Go 1.27.1

Each projection operation scans the entire listed entity count. Values are medians.

| Entities | Existing inline loop | Generic function | Generic method | Method vs inline |
| ---: | ---: | ---: | ---: | ---: |
| 100 | 2.436 µs | 2.563 µs | 2.560 µs | +5.1% |
| 1,000 | 23.93 µs | 24.85 µs | 24.87 µs | +3.9% |
| 10,000 | 241.5 µs | 253.0 µs | 249.5 µs | +3.3% |

Function versus method differences are not statistically significant at the default benchstat 0.05 threshold. Their p-values are 0.810, 0.684, and 0.353. Compared directly with the inline loop, method overhead is significant in these samples, with p-values 0.001, 0.043, and 0.005. These individual comparisons do not adjust for multiple testing.

All projection variants allocate twice per full scan. Both generic variants report 16 B/op. Inline reports 16 B/op at 100 and 1,000 entities and 17 B/op at 10,000. Do not interpret that byte-level difference as an API allocation improvement.

Component Get/increment/Set measures 18.57 ns/op through `Ref[T]` and 18.40 ns/op through the generic entity handle. Both allocate zero bytes. The difference is not significant, p=0.543. This excludes handle construction and query registration.

### Toolchain cost on unchanged APIs

| Case | Go 1.26.5 | Go 1.27.1 | Comparison |
| --- | ---: | ---: | --- |
| Inline scan, 100 entities | 2.412 µs | 2.436 µs | No significant difference |
| Inline scan, 1,000 entities | 23.51 µs | 23.93 µs | No significant difference |
| Inline scan, 10,000 entities | 232.6 µs | 241.5 µs | +3.81%, p<0.001 |
| Function scan, 10,000 entities | 243.5 µs | 253.0 µs | +3.89%, p=0.001 |
| Ref Get/increment/Set | 17.76 ns | 18.57 ns | +4.59%, p=0.005 |

The upgrade shows a small regression in these samples. It is independent of generic method syntax because these rows use the same existing API under both compilers. Do not extrapolate the percentages to production ticks or other CPUs. A production adoption decision with a strict tick budget needs that workload measured too.

## Verification and reproduction

- golangci-lint 2.13.2 analyzes Go 1.27 source but reports 22 findings in existing code. Eighteen gosec findings are in Box2D, and four goconst findings are in Cardinal, lobby, and the physics scenario suite. These findings were not triaged or suppressed. [Lint output](results/lint.txt) and the [old linter failure](results/lint-2.6.2-incompatibility.txt) are retained. CI is not fully green.
- The isolated pulled source passes `TEST_SEED=20260907 go test ./pkg/...` under Go 1.27.1, including Cardinal and physics2d scenarios.
- The reproduction script passed a separate smoke run with both toolchains, the semantic probe, and the statistical summary commands.
- The generic method prototype compiles under Go 1.27.1. The Go 1.26.5 build excludes it using a `go1.27` build constraint.
- The semantic probe observes laziness, early break, `Single` stopping after two results, `Map`/`Filter`/`Single` chaining, preserved IDs, and an empty query. Its timing is not a performance result.
- The shared checkout test run fails in pre-existing local TestWorld work. It imports a missing MessagePack dependency and refers to missing `Component` and `SystemEvent` aliases. No local helper files were changed.

[run.sh](run.sh) creates a fresh snapshot of the recorded base, inserts the prototype files, compiles both toolchains, runs tests, alternates benchmark samples, and writes benchstat summaries. It requires Go toolchain download access, Python 3, Git, and benchstat. It does not change the checkout. `BASE_REV`, `RESULT_DIR`, `SAMPLES`, `BENCHTIME`, and `RUN_TESTS` can override its defaults. An existing `RESULT_DIR/source` is rejected so stale files cannot enter the comparison.

Raw results are [Go 1.26.5](results/go1.26.5.txt) and [Go 1.27.1](results/go1.27.1.txt). Statistical summaries are [function versus method](results/api-comparison.txt), [inline versus method](results/inline-comparison.txt), [component access](results/component-comparison.txt), and [toolchain](results/toolchain-comparison.txt). [Package test output](results/tests.txt) and [semantic observations](results/semantics.txt) retain the verification evidence.

The Model the Domain principle kept registration with World and its declared fields. Exhaust the Design Space prompted comparison of direct owner methods and typed capabilities. Build the Lever produced the rerunnable benchmark script. Prove It Works required compiled prototypes and semantic observations rather than syntax-only sketches.

## Language sources

[Go 1.27 release notes](https://go.dev/doc/go1.27) document method type parameters and the prohibition on generic interface methods. [Generic Methods](https://go.dev/blog/generic-methods) explains the interface restriction. Concrete generic methods do not satisfy interface methods, even if an instantiation would have the desired signature. Ordinary methods on an instantiated generic type remain the option for interface-based capabilities.

The web search response used for language verification is saved locally at `/tmp/cardinal-go127-search.json`.
