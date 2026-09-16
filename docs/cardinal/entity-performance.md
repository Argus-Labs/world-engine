# Entity handle performance

Measured on 2026-09-15 with Go 1.27.1, darwin/arm64, Apple M5 Max,
`GOMAXPROCS=1`. Baseline: `180bfa8352fbbd25d26f736ee5bafeab6f945d2e`, immediately
before the entity-handle migration. Eight alternating before/after samples, 200 ms
per microbenchmark, compared with benchstat. Setup is outside the timed region
except in the registration benchmark.

## Results

No statistically significant slowdown was detected for Get, Set, or the tested full
physics ticks. All other measured steady-state operations improved. This is evidence
for these workloads on one machine, not a guarantee for every application or CPU.

| Operation | Before | After | Time change | Allocations before → after |
| --- | ---: | ---: | ---: | ---: |
| Get | 8.893 ns | 8.976 ns | No significant change | 0 → 0 |
| Set existing component | 9.605 ns | 9.651 ns | No significant change | 0 → 0 |
| Has present component | 8.968 ns | 7.383 ns | -17.7% | 0 → 0 |
| Has absent registered component | 2,241 ns | 6.188 ns | -99.7% | 18 → 0 |
| Has unregistered component | 2,518.5 ns | 7.419 ns | -99.7% | 19 → 0 |
| Has missing entity | 1,281.5 ns | 1.675 ns | -99.9% | 10 → 0 |
| GetByID + component Get | 30.56 ns | 11.88 ns | -61.1% | 1 → 0 |
| Search Create + Destroy, warmed storage | 68.78 ns | 37.93 ns | -44.9% | 1 → 0 |
| Generic Create + Destroy, warmed storage | 68.29 ns | 46.75 ns | -31.6% | 1 → 0 |
| Iterate 100 entities, 1 component | 412.4 ns | 214.9 ns | -47.9% | 2 → 0 |
| Iterate 100 entities, 5 components | 959.9 ns | 216.8 ns | -77.4% | 2 → 0 |
| Iterate 100 entities, 10 components | 1,916 ns | 215.8 ns | -88.7% | 2 → 0 |
| Filter + Limit + Single | 202.90 ns | 79.23 ns | -61.0% | 11 → 4 |
| Remove + re-add component | 153.0 ns | 127.8 ns | -16.5% | 6 → 4 |
| World/registration setup, two archetypes | 1.965 µs | 2.016 µs | **+2.6%** | 25 → 25 |

The startup fixture uses **256 additional bytes** (3,872 → 4,128 B), including the
per-world archetype registry supporting generic Create. This upfront cost is not
paid per entity or per query. The measured startup increase is about 51 ns.

The old API did not expose Has on entity references or generic Create on system
state. Their baseline measurements use the equivalent internal ECS Has and search
Create operations. Other baseline adapters change only component-access syntax.

## Remaining allocations

The API is not allocation-free everywhere:

- Function-valued `Filter(...).Limit(...).Single()` still allocates four objects,
  totaling 100 B in this fixture. Its callbacks and captured mutable state can escape.
- Removing and re-adding a component still allocates four objects, totaling 64 B
  in this fixture, while changing archetypes.
- Growing world storage, new archetypes, component-owned payloads, and error/panic
  paths are outside the zero-allocation steady-state contract.

## Full physics ticks

The existing physics benchmark runs with debug enabled, so it includes debug and
serialization work. Each sample executes 100 ticks after warm-up, using the same
scene and tick sequence on both versions. These are not production-mode numbers.

| Bodies | Before | After | Result |
| --- | ---: | ---: | --- |
| 100 | 154.9 µs/tick | 148.2 µs/tick | No significant timing change, p=0.234 |
| 1,000 | 1.391 ms/tick | 1.383 ms/tick | No significant timing change, p=0.442 |

Both sizes allocate 14 fewer objects per tick. Neither size showed an allocation
increase. Most allocations here belong to the rest of the tick, not entity handles.

## Changes and checks

- Has checks component metadata instead of copying values and constructing absence errors.
- Archetype matching compares bitmap words without allocating an intersection.
- FIFO ID reuse retains its last free slot for repeated create/destroy cycles.
- Query modifiers invoke callbacks directly. Single keeps its captured result together.
- Entity access removes redundant nil checks while preserving documented panic behavior.
- Registration reads field types without decoding names and tags on the success path.

`TestEntity_SteadyStateAllocations` enforces zero allocations for direct operations,
including optional-component misses, lookup, iteration, binding, and warmed generic
creation/destruction. `TestEntity_QueryCompositionAllocations` enforces the measured
four-allocation budget. Bitmap tests cover multiple words, missing words, and trailing
zeros. Existing FIFO, ECS model, snapshot, plugin, and template tests passed.
The suite reported 356 tests with 5 skipped. Focused race and release checks and
Cardinal lint also passed. Lint used a fresh cache because cached staticcheck generic
method facts caused an analyzer panic.

## Reproduce

Requires the repository Go toolchain and `benchstat` on PATH:

```sh
python3 scripts/bench-entity-handles.py \
  180bfa8352fbbd25d26f736ee5bafeab6f945d2e \
  --count 8 --time 200ms --physics
```

The script creates isolated source archives and test binaries under `.scratch`,
prints their directory, alternates execution order, and saves raw samples and
benchstat reports. It does not change branches or include unrelated uncommitted
work. It overlays the named performance implementation files to support rerunning
while developing a fix. The benchmark file contains the exact fixtures and workloads.
