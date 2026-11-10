# NoX CLI

The human terminal interface for NoX Suite. All UI text is US English (EN-US).

**Current scope: Stage 6 — Update and uninstall.** Stage 4's schema forms,
selectors, previews, confirmations, workflows and guarded tool execution are
implemented. The owner has confirmed Yard status and Backend v1.2.2 service-status
execution through Gateway. This does not validate all operations. Yard's current
human interaction design was rejected and further menu work is deferred. See the
[validation record](STAGE_5_VALIDATION.md) for observed results and checks not run.
Stage 6 adds Settings updates, script removal and release/hosting workflows.
No CLI release or hosted installer has been published.

## Try the current source

Requires Go **1.26.0 or newer** for development:

```sh
go mod download
go run ./cmd/nox --version
go run ./cmd/nox
```

Choose **Settings** (selected when unconfigured) with Enter. Use Up / Down to move between fields/actions, Enter to select, and Esc to return. Arrow keys navigate every menu, including Settings and Updates and installation.

1. Enter the Gateway's private base address, for example
   `http://gateway.home.arpa:5711`. Do not append `/mcp` or a product path.
2. Paste the existing Backend personal API key into the masked field.
3. Select **Save and test**. A successful exchange reports the negotiated MCP
   version and tool count. An authenticated empty catalog is valid.
4. The key field is cleared after saving. **Test saved connection** uses the
   existing credential without revealing it. A blank key on Save keeps the saved
   key only when the Gateway origin has not changed.
5. **Remove API key** asks for confirmation and deletes the single native entry.
   Esc returns to the main screen; Esc there or Ctrl+C exits cleanly.

## Navigate app menus

Open **Apps** to load a verified catalog from the configured private Gateway.
Products, names, paths and menu ordering come from discovery; there are no
hardcoded product menus. A configured installation selects Apps at startup.

| Key | Action |
| --- | --- |
| Up / Down, J / K | Select an item |
| Enter / Right | Open a submenu or read-only tool details |
| Esc / Left | Clear a filter, close details, or go back one level |
| Home / End, Page Up / Page Down | Move through long menus |
| `/` | Filter the current menu by label or canonical segment |
| Enter while searching | Keep the filter and return to selection |
| Esc while searching | Clear the filter |
| `R` | Reload the catalog from Gateway |
| `?` | Read menu metadata diagnostics |
| F2 | Open Settings |
| Ctrl+C | Exit and restore the terminal |

Breadcrumbs show the current location. Home uses original NoX block-character art
and a night scene when the terminal has room; submenus and small terminals use a
compact header. Shared violet/teal accents adapt to the terminal's reported light
or dark background without changing its background or font. Selected options use
bold accent text, with blank rows between options when space permits. Menus and details scroll
with the keyboard. Selecting a tool opens its details. Enter starts its input/review flow; selectors
and previews may make read-only requests before final execution is approved.

Gateway context appears on its own **GATEWAY** line. Action messages appear in a
separate bordered panel labeled **SUCCESS**, **WARNING**, **ERROR**, **WORKING** or
**INFO**, with distinct colors for light/dark terminals. Validation and confirmation
messages take priority over earlier status messages. The **KEYS** footer contains
only the controls for the current screen. Messages wrap within the available space;
small terminals use compact notices while keeping the selected item visible.

The catalog is an in-memory snapshot. Browsing, typing and returning to Apps use
that snapshot without network requests. First load and explicit refresh make real
Gateway exchanges; Settings save/test also checks the connection. A snapshot is
not an authorization grant or a live availability guarantee. A rejected key clears
menus and prevents reuse until replacement. Outages keep the configured key and
show a retryable error. There is no authentication polling or idle push channel.

Missing paths, ambiguous paths and unsupported descriptors are identified without
inventing replacement menus or hiding valid actions. Press `?` for diagnostics.
A node can expose both its own action details and submenus. Public Gateway access,
an embedded agent/chat, an incoming MCP server and external MCP aggregation are
outside this CLI's scope. AI clients connect directly to Gateway.

## Run a tool

1. Open **Apps**, select a product and tool, then press Enter on its details.
2. Complete the published input fields. The CLI preserves JSON types and does not
   insert schema defaults. Dynamic selectors use IDs separately from display
   labels; duplicate names are distinguished. Related choices reload when their
   input changes. An empty required selector cannot proceed.
3. Review the arguments and any published preview. Password fields stay hidden.
   Scroll with Up/Down or Page Up/Page Down. Ctrl+E edits inputs and discards the
   previous preview/confirmation. Workflows that already wrote data cannot rewind
   and replay those writes.
4. Enter approves the reviewed action/step. Typed confirmations must match exactly.
   Confirmation arguments are added only where the descriptor requests a binding.
5. Inspect the scrollable result. Structured JSON, ordinary text, tool errors and
   resource information are shown; external resources are never fetched or opened
   automatically. Esc returns to the tool; R reloads menus without replaying it.

| Input | Keys |
| --- | --- |
| Text, password, number | Enter accepts the value |
| Select | Type to filter, arrows to choose, Enter accepts the typed ID |
| Multiselect | Type to filter, Space toggles values, Enter accepts the array |
| Checkbox | Choose Enabled/Disabled, Enter accepts a boolean |
| Textarea or JSON | Enter inserts a line; Ctrl+D accepts |
| Optional field | Ctrl+U omits it rather than inventing an empty/null/false value |
| Previous field | Left in a selector, or Left at the start of a text editor |
| Cancel form/review | Esc |

Options and previews resolve only to available tools from the originating product
and server. Automatic auxiliaries require the published read-only hint; server
permissions remain authoritative. Every business call carries its exact published
fingerprint. Expired catalogs are revalidated before calls, and explicit execution
revalidates all definitions used by the review. A stale definition or lost access
requires refresh and another selection/review. Origin/key changes invalidate old
forms, including changes made by another CLI process.

Each network operation has a two-minute budget, starting when the request starts;
human time at a form/review has no RPC deadline. During execution, Esc or Ctrl+C
requests cancellation and waits for the outcome. A dispatched write may still
complete: an unconfirmed result is shown as **outcome unknown**, never retried.
The next Ctrl+C after completion exits normally. Reported progress is displayed
when the transport/peer supplies it; there is no idle polling or SSE subscription.

Successful or uncertain writes expire the local catalog snapshot; subsequent
selectors read fresh data. Tool failures stop workflows before later steps.
MCP multi-round-trip input requests are explicitly unsupported and are not
resubmitted automatically. Binary attachments are not opened by this terminal.

Current local form limits are explicit: external schema references and recursive
or more than 32 nested object levels are blocked; parameter controls inside
variable array elements are unsupported (arrays/objects can be entered as whole
JSON fields); selectors are limited to 5,000 options; and text fields to 1,048,576
characters. JSON numeric lexemes are retained in arguments, bindings and output.
Local validation supports 64-bit integers and finite floating-point numbers;
values beyond its validation range are rejected instead of rounding integers.
Output-schema failures retain the returned result for inspection and do not imply
that a write was rolled back. Unsupported paths show a diagnostic, leaving other
tools available.

## Private Gateway access

Use a private LAN or Tailscale/Twingate route. The CLI computer does not need to
share the services' host or subnet. Both HTTP and HTTPS are accepted for private
Gateway origins; HTTPS retains normal system certificate verification.

The transport permits private IPv4/IPv6, loopback, and the shared `100.64.0.0/10`
range used by supported VPN addressing. It rejects unspecified, public,
link-local and multicast destinations. Hostname answers are checked and pinned
to the actual dialed address; a mixed private/public answer is rejected. It does
not follow redirects or use environment HTTP proxies to forward the key.
Public Gateway addresses/tunnels and application path prefixes are unsupported.
These client checks do not replace the Gateway's own authentication or firewall.

The check uses official MCP and NoX discovery over Streamable HTTP JSON, with a
20-second overall connection/discovery budget and no standalone SSE subscription.
No product socket, Docker API, enrollment endpoint or direct product address is
used. A rejected key is distinguished from network/TLS failures, tool permission
errors and outages. Rejection blocks reuse until a new key is entered; there is
no push or polling invalidation while the CLI is idle.

## Credentials and local data

| System | Application directory | Secret store |
| --- | --- | --- |
| Windows | `%LOCALAPPDATA%\nox-cli` | Windows Credential Manager |
| macOS | `~/Library/Application Support/nox-cli` | Keychain |
| Linux | `${XDG_DATA_HOME:-$HOME/.local/share}/nox-cli` | Secret Service |

`config.json` contains only the origin, version/mode and the fixed secret reference
`nox-cli/api-key`. The native slot uses service `nox-cli` and account `api-key`
(Windows represents that pair as `nox-cli:api-key`). Replacement updates the same
slot; there are no suffixed key entries or retries with an older credential.
The secret is origin-bound inside the native vault, so interrupted configuration
changes cannot send a saved key to another Gateway origin.

The current Backend key format is `noxsync_` plus 32 base64url bytes: 51 characters.
The masked field validates format without echoing the value. The CLI never offers
Reveal, Copy or Export, accepts no API key argument, and writes no key to settings,
logs or diagnostics. It retrieves the secret in memory for authentication.

If the native vault is unavailable/locked, unlock it or explicitly choose
**Use API key only for this session**. There is no plaintext fallback. Session-only
settings retain no key or secret reference; the key is forgotten when the process
ends. A previously persisted slot is not silently reused in this mode. Selecting
session-only mode does not claim to delete an inaccessible previous vault entry;
Remove API key remains the explicit deletion action.

Failed candidate validation does not replace existing settings/credentials.
Persistence failures attempt to restore the previous value in the same slot.
Cancellation prevents a pending connection check from saving settings.

## Installation

Installers, updates, uninstallers and release packaging are implemented in source.
**No CLI release or installer domain has been published by this work.** The commands
below become usable after the owner publishes a release and the installer endpoints.
End users need no Go, Node, Rust, Python, Docker, administrator account or manual
archive extraction.

macOS / Linux (Bash or Zsh):

```sh
curl -fsSL https://cli.mapherez.com/install.sh | sh
```

Windows PowerShell:

```powershell
& ([scriptblock]::Create((irm https://cli.mapherez.com/install.ps1)))
```

The script downloads the official GitHub release, verifies its SHA-256 checksum,
and extracts only the expected native executable. It activates `nox` directly in
the application directory listed above and makes an owned, idempotent PATH change.
Open a new terminal after installation. Windows changes only the user PATH;
macOS/Linux preserve existing Bash/Zsh profile content. A modified PATH block,
foreign `nox` command, linked target or unowned executable causes a diagnostic.
Shell profiles and the application root must share a filesystem for atomic edits.

## Updates

Open **Settings -> Updates and installation**. Stable release builds check public
release metadata asynchronously at startup, at most once per 24 hours. A newer
version appears in the status line. **Check for updates** performs an explicit
refresh; offline checks never block Suite access. Source `dev` builds do not
check automatically and are never replaced by the updater.

Choose **Install update**, review the version, and confirm. No update is silent.
The updater rechecks the release, downloads version-pinned assets, verifies their
checksum and embedded version, and replaces only the installer-owned executable.
Settings and the single native credential entry are retained. Release requests
use a separate HTTP client with no Gateway API key, cookies or environment proxy.

NoX closes after a successful update; run `nox` again. Windows uses a short-lived
PowerShell helper to wait for the current process to exit before replacement.
There is no update service. The application cannot start an update during an active
business operation, and no operation is replayed after restart.

Downloads, backup/journal files and the Windows helper live only in `nox-cli/install/`.
They are removed after success, failure or normal cancellation. An abrupt process
termination leaves an owned recovery record: the next startup or installer run
recovers the transaction and cleans its staging. Failed replacement preserves the
previous executable; unresolved recovery retains its backup rather than deleting it.
If another installation/update is finishing, wait briefly and run `nox` again.
`update-check.json` contains only check time/version; Windows `update-result.json`
contains a non-secret outcome. Neither contains a credential.

## Uninstall

Close other NoX CLI processes. By default, removal retains `config.json` and the
single native credential so a later reinstall can reuse them.

macOS / Linux:

```sh
curl -fsSL https://cli.mapherez.com/uninstall.sh | sh
```

Windows PowerShell:

```powershell
& ([scriptblock]::Create((irm https://cli.mapherez.com/uninstall.ps1)))
```

To explicitly remove configuration and the single owned credential as well:

```sh
curl -fsSL https://cli.mapherez.com/uninstall.sh | sh -s -- --purge
```

```powershell
& ([scriptblock]::Create((irm https://cli.mapherez.com/uninstall.ps1))) -Purge
```

The uninstaller bootstraps a freshly verified release payload, so the installed
executable need not work. It removes only recorded executable/PATH ownership and
known maintenance records; unrelated files are retained. Empty shell profiles
created by the installer are removed; pre-existing profiles are preserved. Edited
PATH blocks must be restored before removal rather than silently overwritten.
Full cleanup deletes only the `nox-cli/api-key` native slot. If the vault refuses
deletion, the error is reported; no plaintext export or fallback is created.
Open a new terminal afterward. Scripts do not recursively erase the application
root: it disappears only when empty. Full cleanup can also run after a prior
keep-settings uninstall.

## Release and installer publication

Release assets are six archives containing exactly one native executable:
`nox-cli_<darwin|linux>_<amd64|arm64>.tar.gz` with `nox`, and
`nox-cli_windows_<amd64|arm64>.zip` with `nox.exe`. The release also contains all four
installer/uninstaller scripts and a complete `checksums.txt`.

The **Release** workflow runs on an owner-pushed stable `vX.Y.Z` tag. It reuses
existing native CI checks and builds the six packages in place of that run's
compile-only cross-build. It does not add any Suite-wide integration gate.
The selected tag's version/commit/date are embedded in each executable. All assets
are verified and uploaded to a draft before publication; a failed upload does not
publish a partial release. No local command here creates a commit, tag or release.

The separate **Publish installer endpoints** workflow is manually triggered with
a published stable tag. It verifies the four scripts against that release's
checksums and deploys static files through GitHub Pages. One-time owner setup:
select **GitHub Actions** as the Pages source, configure `cli.mapherez.com` as its
custom domain, point its DNS at this repository's GitHub Pages host, and enable
HTTPS. Then run that workflow for the desired release. No domain, tunnel or hosted
site is created by running the CLI or its source checks.

For local packaging verification only (not an end-user installation method):

```sh
python3 scripts/build_release.py --version vX.Y.Z --output dist
python3 scripts/build_release.py --output dist --verify
```

The output directory must not already exist. Build outputs are ignored by Git.
Publishing the scripts/domain and release remains an owner action; these source
changes do not establish that an installer URL is live or certify a platform's
native behavior from cross-compilation alone.

## Development and checks

```sh
go mod verify
go mod tidy -diff
go vet ./...
go test -mod=readonly ./...
go test -race -mod=readonly ./...
python3 scripts/check_foundation.py
python3 scripts/check_foundation.py --cross-build
python3 -m unittest discover -s scripts -p 'test_*.py'
```

Build a developer binary with `go build -trimpath -o dist/nox ./cmd/nox` (use
`dist/nox.exe` on Windows). Source builds report `dev`, `unknown` commit and
`unknown` build date. Build values target
`github.com/mapherez/nox-cli/internal/buildinfo.Version`, `.Commit` and `.Date`.
This does not install the binary, set PATH or create a release.

Implementation boundaries:

- `cmd/nox`, `internal/command`, `internal/buildinfo`: process/build identity.
- `internal/tui`: persistent interface, masked Settings, local search, details and terminal lifecycle.
- `internal/navigation`: generic product/path tree and per-action metadata diagnostics.
- `internal/interaction`: schema forms, pointer bindings, dependency invalidation and reviewed execution/workflows.
- `internal/config`: non-secret settings and per-user path policy.
- `internal/credentials`: the single native credential slot and key-format checks.
- `internal/gateway`: official MCP discovery and the authenticated private transport.
- `internal/setup`: candidate validation, persistence, replacement and rejection state.
- `internal/install`: owned activation, transactional replacement/recovery, native PATH changes and removal.
- `internal/releases`: public metadata, pinned archive download, checksum and extraction limits.
- `internal/maintenance`: bounded release checks and explicit updates, independent of Suite credentials.

Tests use disposable directories, small official MCP/discovery peers, and explicit installer
handoff fixtures; they are not fictitious Suite products. Go installer tests
exercise actual file/PATH-edit logic in isolated targets; script fixtures check
verified download/handoff/cleanup. They do not install into the developer's real
profile. The native vault test is opt-in, uses a unique `nox-cli-test-*` slot,
and deletes it afterward; it never touches `nox-cli/api-key`:

```sh
NOX_CLI_TEST_NATIVE_VAULT=1 go test ./internal/credentials -run TestNativeVaultReplacement -count=1 -timeout=30s
```

[CI](.github/workflows/ci.yml) runs native Go/process checks on Linux, Windows and
macOS, POSIX installer and PTY checks, Windows PowerShell installer checks, native
vault tests, and six-target compilation. The Linux vault check uses a disposable
encrypted Secret Service inside its own D-Bus session. The race check runs on Linux.
The native Windows terminal/ConPTY restoration check remains separate from POSIX
PTY evidence. Compiling another target does not certify its runtime behavior.
CI does not publish. The tag-triggered Release workflow and owner-triggered
installer-hosting workflow perform the separately documented publication steps.

Local/remote/installed evidence is recorded separately in the
[implementation plan](IMPLEMENTATION_PLAN.md). The [Stage 5 record](STAGE_5_VALIDATION.md)
contains the owner's two confirmed status operations and the checks not reported.
Yard menu work remains deferred. Local Stage 6 checks use only disposable state;
they do not claim an actual published release or live hosted installer endpoint.
