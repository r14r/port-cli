# port-cli

Native Go CLI for maintaining MacPorts and migrating Homebrew formulae on macOS. The executable is named `port-cli`.

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
CGO_ENABLED=0 go build -trimpath -o dist/port-cli .
sudo install -m 755 dist/port-cli /usr/local/bin/port-cli
```

## Usage

```sh
port-cli --version
port-cli update
port-cli migrate --dry-run
port-cli migrate --ui
port-cli migrate --only llama.cpp --dry-run
port-cli migrate --only llama.cpp --yes
port-cli migrate --exclude git --yes
port-cli migrate --yes
```

Migration defaults to read-only. Only `--yes` permits installation and removal. Run as your normal user; the CLI invokes sudo only for MacPorts operations. Put /opt/local/bin and /opt/local/sbin before Homebrew on PATH when you are ready to switch.

## Scheduled updates (cron)

```sh
port-cli update --crontab
```

This command writes no terminal output after successful log initialization. It
creates one private, timestamped log per invocation in
`$HOME/logs/port-cli/update-YYYYMMDD-HHMMSS-*.log`, recording both stdout and stderr,
the start/end time, commands and outcome. Failures return a nonzero exit code.
A file lock prevents concurrent cron-mode updates; overlapping invocations log
a failure and exit. Manual updates are not covered by this lock. Logs are retained
until you remove them. If logging itself cannot be initialized, an error is
reported to stderr.

Cron mode uses a fixed standard MacPorts prefix (`/opt/local`), a minimal PATH,
no stdin, and non-interactive commands:

```sh
/usr/bin/sudo -n /opt/local/bin/port -N selfupdate
/usr/bin/sudo -n /opt/local/bin/port -N upgrade outdated
```

Run as your normal user. Unattended updates require sudo authorization for these
two exact commands without a password. If this is acceptable for your system,
use `sudo visudo` to configure the following rule, replacing YOUR_LOGIN with
your macOS short username (`id -un`). This grants privileged package updates;
review and trust your MacPorts sources and configuration.

```sudoers
YOUR_LOGIN ALL=(root) NOPASSWD: /opt/local/bin/port -N selfupdate, /opt/local/bin/port -N upgrade outdated
```

Do not grant unrestricted passwordless sudo. If authorization is missing,
the update fails immediately and records the error rather than prompting.

Test the actual command manually and inspect the log before scheduling it:

```sh
port-cli update --crontab
echo $?
ls -lt "$HOME/logs/port-cli"
```

For example, run daily at 03:00 via your user crontab (`crontab -e`):

```cron
0 3 * * * /usr/local/bin/port-cli update --crontab
```

No crontab entry is installed automatically. The Mac must be awake for cron
to run on schedule. Automatic package upgrades can affect running services;
choose a suitable maintenance window.

`just update-crontab` builds and runs the same mode for testing.

## Interactive migration

Run `port-cli migrate --ui` in a terminal (or `just migrate-ui`).
For detailed scan progress, run `port-cli migrate --ui --details` or
`just migrate-ui-details`. It shows actual Homebrew inventory and MacPorts
catalog counts, package matching, per-candidate dependency checks and the
final status counts before opening the selector. The progress remains in
the original terminal screen after leaving the selector. The same flag
works with non-interactive migration modes and selection revalidation.
Without it, the existing compact output is unchanged.

All installed formulae and casks are shown. No packages are initially selected.
Use Up/Down (or j/k), Space to toggle an eligible row, a to toggle all eligible
rows, n/p to page, Tab to focus **Migrate Selected**, Enter to activate it,
and q to cancel. The focused package's reason is displayed below the table.

READY and REVIEW can be selected. REVIEW indicates that activation can be
checked but a predefined executable smoke test is missing. KEEP, CASK and
BLOCKED remain visible but disabled. The UI computes dependency eligibility
assuming all otherwise eligible formulae could be selected; a dependency can
therefore be selectable in the UI when it was BLOCKED in a normal bulk dry run.
Select its eligible Homebrew dependents as well. If the chosen subset is not
dependency-safe, the complete selection is rejected before any changes.

After pressing Migrate Selected, the terminal returns to normal mode, the
selection is replanned and displayed with its verification level. Type y
to confirm or anything else to cancel. Migration then uses the existing
install-first, verification, backup and dependency guards. Actual installation
failures can still produce a partial migration. No protected formula or cask
can be selected. UI requires an interactive terminal and the system stty
utility; no extra Go dependencies are needed.

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
