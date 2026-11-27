# port-cli

Native Go CLI for maintaining MacPorts and migrating Homebrew formulae on macOS. The executable is named `ports-cli`.

## Requirements

- macOS with MacPorts installed (usually `/opt/local`).
- Homebrew for migration commands only.
- Go 1.22 or newer, using a Go release compatible with your macOS, for building.
- Optional: [just](https://github.com/casey/just) for development commands.

No third-party Go modules. Intel and Apple Silicon builds are supported. A compiled binary does not require Go or Ruby at runtime (the package managers retain their own dependencies).

## Build and install

```sh
git clone https://github.com/r14r/port-cli.git
cd port-cli
just ci
just install
```

Without just:

```sh
go test ./...
go vet ./...
mkdir -p dist
CGO_ENABLED=0 go build -trimpath -o dist/ports-cli .
sudo install -m 755 dist/ports-cli /usr/local/bin/ports-cli
```

## Usage

```sh
ports-cli --version
ports-cli update
ports-cli migrate --dry-run
ports-cli migrate --only llama.cpp --dry-run
ports-cli migrate --only llama.cpp --yes
ports-cli migrate --exclude git --yes
ports-cli migrate --yes
```

Migration defaults to read-only. Only `--yes` permits installation and removal. Run as your normal user; the CLI invokes sudo only for MacPorts operations. Put /opt/local/bin and /opt/local/sbin before Homebrew on PATH when you are ready to switch.

## Migration policy

The CLI reads the local MacPorts catalog, installed port state, installed Homebrew formulae and casks, pinned packages, and services. Exact names and a small alias map are used; no fuzzy matching.

- READY: eligible in dependency order, but build and feature parity are not guaranteed.
- BLOCKED: an installed dependent must remain or cannot be resolved.
- REVIEW: a matching port exists but there is no predefined executable smoke test; review it and use an explicit `--only NAME` selection if appropriate.
- KEEP: excluded, pinned, service, sensitive runtime, keg-only, third-party tap, missing match or failed dependency check.
- CASK: manual migration only.

All installed formulae and casks are reported. Runtime, database, shell and infrastructure packages are protected even with --only. There is no force removal and no automatic dependency cleanup. Bulk execution uses an allowlist of smoke-tested CLI replacements. Explicit --only for a REVIEW package checks activation but does not prove functional parity. Install the replacement, verify activation and its absolute-path executable when a smoke test is defined, recheck Homebrew dependents, pins and services, then uninstall. Dependents are processed before their dependencies. Failures prevent removal if verification fails, and later dependency checks are repeated.

## Recovery

Before mutations a timestamped inventory is saved to ~/.local/state/ports-cli/ containing a Brewfile, installed formula metadata, version inventory, installed ports, plan and migration log. This is not a binary, configuration or data backup. Review external consumers such as virtual environments, manually linked binaries and scripts before removal. A Brewfile is an inventory, not an exact version rollback. Consult it and the log if recovery is needed. MacPorts build failures on older macOS releases are possible.

## Development

```sh
just doctor
just fmt
just test
just lint
just build
just build-macos
just ci
just version
```

Tests use fake package managers and do not install or remove packages. No live macOS package mutation is performed by tests.

References: [Homebrew manpage](https://docs.brew.sh/Manpage), [MacPorts Guide](https://guide.macports.org/).
