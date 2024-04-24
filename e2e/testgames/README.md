# Testgames

This folder contains world-engine games used for testing, benchmarking, and manual tests.

## Directories
- **game**: Contains a world-engine game used for end-to-end tests.
- **gamebenchmark**: Contains a world-engine game for benchmarks.

## Manual Testing

This directory contains configuration files and a symbolic link to support development and testing of the `game` project using `world cardinal dev`. 
This is useful because changes to `world-engine/cardinal` can be viewed right away without building any docker containers.

- **world.toml**: Configures parameters for `world cardinal dev`.
- **cardinal**: A symbolic link pointing to the `game` directory, ensuring `world cardinal dev` references the correct code.

Note: There is no `docker-compose.yml` file, so `world cardinal start` will fail.

## TODO
Once [WORLD-1078](https://linear.app/arguslabs/issue/WORLD-1078/world-cardinal-dev-has-the-cardinal-directory-hard-coded) is resolved, world.toml can be updated to point directly to the `game` directory, and the `cardinal` symlink can be removed.


