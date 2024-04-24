
This directory contains a docker-compose.yml as well as a world.toml file. These files make it easy to run the `game` project using `world cardinal dev`. 

Running `world cardinal dev` here is useful because it allows for manual testing and rapid iteration of the end-to-end game without having to run any docker containers.

TODO: This directory contains the symbolc link `cardinal` that points to the `game` directory. This is so `world cardinal dev` runs the correct code. Currently, that `cardinal` directory is hard coded in the world-cli tool. Once [WORLD-1078](https://linear.app/arguslabs/issue/WORLD-1078/world-cardinal-dev-has-the-cardinal-directory-hard-coded) has been implemented, the cardinal directory will be configurable via world.toml, and the symlink can be removed.
