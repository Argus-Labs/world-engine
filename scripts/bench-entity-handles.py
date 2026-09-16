#!/usr/bin/env python3
"""Compare pre-handle Cardinal with the current entity implementation.

Usage: python3 scripts/bench-entity-handles.py BASELINE [--count 6] [--time 200ms]
Requires Go and benchstat. Archives are disposable copies; no checkout is changed.
The baseline adapter only changes benchmark API spelling to perform the same work.
"""
import argparse
import io
import os
import re
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile


def run(args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("baseline")
    parser.add_argument("--count", type=int, default=6)
    parser.add_argument("--time", default="200ms")
    parser.add_argument("--physics", action="store_true", help="also compare full physics ticks (debug enabled)")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    scratch = root / ".scratch"
    scratch.mkdir(exist_ok=True)
    output = Path(tempfile.mkdtemp(prefix="entity-bench-", dir=scratch))
    print(f"Results: {output}", flush=True)
    env = dict(os.environ, GOMAXPROCS="1", TEST_SEED="1", GOWORK="off")
    benchmark = (root / "pkg/cardinal/entity_perf_internal_test.go").read_text()
    benchmark = benchmark.split("// Keep successful handle operations", 1)[0]
    # Restore the old query signatures before adapting component access.
    baseline = re.sub(r"(\w+) := state.Entities.Create\(\)", r"_, \1 := state.Entities.Create()", benchmark)
    baseline = baseline.replace("found, err := state.Entities.Iter()", "_, found, err := state.Entities.Iter()")
    baseline = baseline.replace("func(e Entity) bool", "func(_ EntityID, e Entity) bool")
    baseline = baseline.replace("entity.ID()", "entity.Position.entity")
    baseline = baseline.replace("found.ID()", "found.Position.entity")
    baseline = baseline.replace("sum += id.ID()", "sum += id")
    baseline = baseline.replace("WithComponent[", "Ref[")
    baseline = baseline.replace("entity.Set(Position3D", "entity.Position.Set(Position3D")
    baseline = baseline.replace("entity.Set(Velocity3D", "entity.Velocity.Set(Velocity3D")
    baseline = baseline.replace("entity.Remove[Velocity3D]()", "entity.Velocity.Remove()")
    baseline = baseline.replace("e Entity", "e perfArchetype")
    baseline = baseline.replace(".Get[Position3D]()", ".Position.Get()")
    for kind in ("Position3D", "Velocity3D", "Inventory", "NetworkSync"):
        baseline = baseline.replace(f"entity.Has[{kind}]()", f"state.world.world.Has[{kind}](entity.Position.entity)")
    baseline = baseline.replace("entity := state.Entity(999)",
        "entity := struct{ Position Ref[Position3D] }{Position: Ref[Position3D]{ws: state.world.world, entity: 999}}")
    baseline = baseline.replace("entity.Destroy()", "state.Entities.Destroy(entity.Position.entity)")
    baseline = baseline.replace("warmup.Destroy()", "state.Entities.Destroy(warmup.Position.entity)")
    baseline = baseline.replace("entity := state.Create[perfArchetype]()", "_, entity := state.Entities.Create()")
    for label, revision in (("before", args.baseline), ("after", "HEAD")):
        dest = output / label
        dest.mkdir()
        archive = run(["git", "archive", revision], cwd=root, capture_output=True).stdout
        with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
            tar.extractall(dest, filter="data")
        if label == "after":
            # Include these files' uncommitted performance fixes during development.
            for name in ("entity.go", "system.go", "internal/ecs/ecs.go", "internal/ecs/archetype.go", "internal/ecs/world_state.go"):
                shutil.copyfile(root / "pkg/cardinal" / name, dest / "pkg/cardinal" / name)
        (dest / "pkg/cardinal/entity_perf_internal_test.go").write_text(baseline if label == "before" else benchmark)
        run(["go", "test", "-c", "-o", str(output / f"{label}.test"), "./pkg/cardinal"], cwd=dest, env=env)
        if args.physics:
            run(["go", "test", "-c", "-o", str(output / f"{label}-physics.test"), "./pkg/plugin/physics2d/test"], cwd=dest, env=env)
    # Alternate order to reduce warm-up and thermal bias. Never run benchmarks concurrently.
    for sample in range(args.count):
        order = ("before", "after") if sample % 2 == 0 else ("after", "before")
        for label in order:
            with (output / f"{label}.txt").open("a") as out:
                run([str(output / f"{label}.test"), "-test.run=^$", "-test.bench=^BenchmarkEntity",
                     "-test.benchmem", f"-test.benchtime={args.time}", "-test.cpu=1"],
                    cwd=output / label, env=env, stdout=out)
        print(f"Sample {sample + 1}/{args.count}", flush=True)
    if args.physics:
        for sample in range(args.count):
            order = ("before", "after") if sample % 2 == 0 else ("after", "before")
            for label in order:
                with (output / f"{label}-physics.txt").open("a") as out:
                    run([str(output / f"{label}-physics.test"), "-test.run=^$",
                         "-test.bench=^BenchmarkStep$/^Bodies_(100|1000)$", "-test.benchmem",
                         "-test.benchtime=100x", "-test.cpu=1"], cwd=output / label, env=env, stdout=out)
            print(f"Physics sample {sample + 1}/{args.count}", flush=True)
    with (output / "comparison.txt").open("w") as out:
        run(["benchstat", str(output / "before.txt"), str(output / "after.txt")], stdout=out)
    print((output / "comparison.txt").read_text())
    if args.physics:
        with (output / "physics-comparison.txt").open("w") as out:
            run(["benchstat", str(output / "before-physics.txt"), str(output / "after-physics.txt")], stdout=out)
        print((output / "physics-comparison.txt").read_text())


if __name__ == "__main__":
    main()
