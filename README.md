# Golang Template

This template adds basic settings for starting a golang project.

## Included

### flake.nix and .envrc

A nice way to setup tooling. Run `direnv allow` to enable envrc to load settings.

**Note:** Requires that you are using nix. If you are on Mac, use the [Determinate Systems](https://determinate.systems/nix-installer/) nix installer. 

### Makefile

A standard Makefile with targets for build, lint and test. See `make help` for targets.

**Note:** the Makefile assumes that the binary to build is the basename of the current repo.

### .golangci.yml

A strict golangci-lint file that is based on "Effective Go". It is designed as a way to stop AI from writing slop.

### .gitignore

Standard golang .gitignore file

## WARRANY

There is no warranty for this template. Use at your own *risk*. There be dragons.
