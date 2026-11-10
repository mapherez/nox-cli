# NoX CLI technical reference

This document describes the implemented CLI architecture and operational behavior. It is a maintained reference, not an implementation plan or a record of development checkpoints. For the product overview and normal use, see [README](README.md).

## Scope and architecture

The installed command is `nox` (`nox.exe` on Windows). It is one native Go application using Bubble Tea, Bubbles and Lip Gloss for the terminal, the official MCP Go client for protocol exchange and NoX MCP for shared discovery/descriptor contracts. Dependency versions are recorded in `go.mod` and `go.sum`.

```text
Human -> nox -> private Gateway /mcp -> authorized Suite product
AI client ----> Gateway /mcp -------> authorized Suite product
```

CLI has no incoming listener, Docker integration, product socket access, product registry or Google login provider. It does not expose an MCP server or embed an agent/chat or external-server aggregator. A product's identity, labels, tool paths and schemas are received data; production code does not select behavior by product name.

| Package | Responsibility |
| --- | --- |
| `internal/command` | Process startup, supported arguments and internal installer handoffs. |
| `internal/tui` | Terminal state, keyboard navigation, forms and result presentation. |
| `internal/navigation` | Generic menu tree built from verified product metadata. |
| `internal/interaction` | Schema fields, pointer bindings, options, previews and sequential workflows. |
| `internal/setup` | Validated connection/configuration changes and credential ownership. |
| `internal/gateway` | Private HTTP transport, MCP negotiation, catalogs and guarded calls. |
| `internal/config`, `internal/credentials` | Non-secret settings and the native credential store. |
| `internal/install` | Owned activation, PATH changes, update recovery and removal. |
| `internal/releases`, `internal/maintenance` | Public release metadata, verified downloads and requested updates. |

The interactive process requires a terminal. `--help` and `--version` work without one; unsupported command arguments fail explicitly. Exit and interruption restore terminal state. Network work is asynchronous so it does not block keyboard handling.

## Gateway connection boundary

Settings accepts a configured HTTP(S) origin and derives only its `/mcp` endpoint. An origin cannot include credentials, an application prefix, a query or a fragment. There is no configured default Gateway address.

The actual resolved destination is checked for each connection. Supported targets include private addresses, loopback, IPv6 private/loopback addresses and the shared address range used by private VPNs. IPv4-mapped IPv6 is normalized. A DNS answer containing a public destination is rejected rather than tried after a private result. The CLI and Gateway need not be on the same subnet.

HTTP is supported for private access. HTTPS retains normal certificate verification; there is no insecure fallback. Redirects and environment HTTP proxies cannot change the credential's destination. The transport attaches the personal key only to the configured origin's MCP path.

Release/update HTTP uses a separate client and the official-host allowlist. It never receives the Gateway key or product arguments. Public release downloads are independent of the private-only Gateway policy.

## Files and credential ownership

| Platform | Application root |
| --- | --- |
| Windows | `%LOCALAPPDATA%\nox-cli` |
| macOS | `~/Library/Application Support/nox-cli` |
| Linux | `${XDG_DATA_HOME:-$HOME/.local/share}/nox-cli` |

The executable lives directly under that root. `config.json` stores the origin, configuration schema, credential reference and session-only selection; it contains no API key. `install-state.json` records installer ownership, executable hash and owned PATH hooks. `update-check.json` caches public metadata; `update-result.json` carries a bounded update outcome.

The `install/` subdirectory is the managed area for downloads, extraction, candidate checks, update journals/backups and short-lived helpers. Ordinary success, failure or cancellation cleans owned staging. An abrupt interruption is recovered or cleaned on a later invocation. Unknown files and changed/symlinked targets are not recursively removed. POSIX directories are private and sensitive files use restrictive permissions.

The credential identity is always service `nox-cli`, account `api-key`, referenced as `nox-cli/api-key`; Windows represents the pair as `nox-cli:api-key`. The native adapters use Windows Credential Manager, macOS Keychain and Linux Secret Service. Replacement updates this one entry instead of creating suffixed entries.

The stored secret contains the configured Gateway origin alongside its key. Configuration changes cannot silently reuse it for another origin. The current Backend key-format check expects `noxsync_` plus 32 base64url bytes, totaling 51 characters. This is a local format check, not authentication or a promise about future key formats.

Key input is masked. The UI does not reveal, copy or export it, and accepts no key command-line argument. A candidate is tested before replacing saved settings/credentials. Errors do not echo its value. Gateway responses and terminal messages are redacted and sanitized; authentication still requires the key in process memory.

If native storage is unavailable, explicit session-only mode keeps the key only in memory. There is no plaintext fallback or silent reuse of an inaccessible old entry. Session-only mode does not claim to delete an older stored credential; Remove API key is the explicit removal action. Normal connection failures preserve the previous saved configuration.

## Discovery and execution

The CLI negotiates MCP and NoX discovery over Streamable HTTP JSON. It does not depend on an SSE subscription. Catalogs and definitions are cached in memory and partitioned by the configured origin and credential context; no persistent tool database is maintained.

Product presentation and `nox/gateway` provenance establish product/server identity. Canonical definitions supply the `_meta.cli` paths and separate interaction descriptor. Ordinary AI-facing `tools/list` is not used to reconstruct missing human descriptors. Ambiguous paths, unavailable tools, missing references and unsupported required features are reported rather than guessed.

Fields derive from published schemas and descriptor controls: text, password, number, select, multiselect, checkbox, textarea and JSON. Values preserve their JSON types. Defaults are not inserted automatically. Dynamic choices bind published values separately from display labels; duplicate labels remain distinct. Empty required selections cannot execute an operation. Changed dependencies invalidate later values and prior review state.

Options, previews and workflow references must resolve within the originating product/server and remain authorized. Automatic auxiliary calls require read-only declarations. RFC 6901 pointers and explicit bindings provide data flow; ordinary text is not parsed into authority or executable workflow instructions.

Confirmation binds the arguments, preview, used definition fingerprints and current workflow step. Editing invalidates the previous confirmation. A confirmation argument is added only if the descriptor declares that binding. Completed workflow writes cannot be rewound or replayed by editing.

Business calls carry the selected fingerprint. Expired catalogs and explicit execution revalidate the relevant definitions and authorized view. A changed definition, removed access or origin/key change discards reusable execution state. The CLI does not substitute another tool or infer its identity from an alias string.

Discovery/connection checks have a 20-second budget. Each guarded tool call has a two-minute budget beginning when the call starts; time spent editing a form does not consume it. Gateway and downstream tools can impose tighter limits. Transport responses are bounded, and selectors are limited to 5,000 options. Structured numbers are restored from the actual response using exact JSON number decoding, preserving large integers. Untrusted labels/content cannot emit arbitrary terminal-control sequences or execute shell commands.

Canceling a call does not prove that a write was rolled back. A lost result after a possible write reports an unknown outcome. Tool calls and unsupported multi-round-trip requests are never automatically replayed after cancellation, reconnect, stale discovery or key replacement.

## Authentication failure and long sessions

Backend/Gateway remain responsible for authorization, renewal and revocation. The CLI learns of key rejection through an actual Gateway exchange, clears credential-bound catalogs/forms and requests a replacement. There is no idle authentication poller or push-invalidation channel, and no extra authorization call for every menu keypress.

A tool-specific denial, domain error, timeout, unavailable product or Gateway outage is not treated as proof that the entire key is invalid. Network errors preserve the key while preventing unverified execution. Local navigation uses the loaded snapshot without a request; discovery refresh, options, previews and tool calls use the network.

## Installation, update and removal

The supplied scripts install a verified native executable per user without elevation. End users do not need the development toolchain or manual archive extraction. Windows uses the user PATH; supported POSIX shells use marked profile additions. Reinstall preserves ownership, avoids duplicate PATH entries and refuses unrelated existing commands or modified installer hooks.

Public update metadata is checked asynchronously with a 24-hour cache. Updates require an explicit user action and a managed installed executable; source builds are not replaced. Downloads are pinned to the selected release, bounded, checksum-verified and extracted only as the expected regular executable. The candidate's embedded identity is checked before activation.

Activation uses an ownership/hash check and a recovery journal. Failed preparation leaves the prior executable intact. Windows uses a short-lived windowless helper to wait for the running process before replacement and cleanup. No updater service is installed. Update recovery concerns installer files only; it cannot resume or replay business operations.

Uninstall scripts bootstrap a verified payload independently of the installed executable. Default removal keeps settings and the credential. Explicit `--purge` on POSIX or `-Purge` on PowerShell removes the owned configuration and native credential entry too. An explicit later purge after keep-settings removal is supported. Unrelated files, vault entries and shell configuration are preserved.

## Development and publication

Build/runtime dependencies are locked in `go.mod` and `go.sum`. Existing checks include Go tests/vet, isolated terminal/process checks, installer script tests and native platform CI. Tests use disposable state; native-vault tests are opt-in and use a separate test credential identity.

Useful source checks include:

```sh
go test -mod=readonly ./...
go vet ./...
python3 scripts/check_foundation.py
npm run test:release
```

The [CI workflow](.github/workflows/ci.yml) defines the required platform checks. Cross-compilation is not native runtime evidence. These commands are checks, not release or installation commands.

`npm run release` uses the clean committed checkout to update `package.json`, create an annotated release tag and push branch/tag atomically. Pre-commit failures restore the manifest; later failures retain the local commit/tag and print recovery instructions. The [release workflow](.github/workflows/release.yml) runs existing CI, packages the supported native targets, verifies assets and uploads a complete draft before publication. Existing published releases are not overwritten.

The [installer publication workflow](.github/workflows/installers.yml) is a separate manual GitHub Pages deployment. Enable GitHub Actions as the repository's Pages source, configure the chosen installer domain/DNS and HTTPS when using a custom domain, then select a published stable release in that workflow. It verifies the scripts against their release checksums before deploying them. Source builds and checks do not publish endpoints.

## Shared references

The current shared [Gateway contract](https://github.com/mapherez/nox-mcp/blob/master/Documents/GATEWAY_CONTRACT.md), [SDK discovery reference](https://github.com/mapherez/nox-mcp/blob/master/Documents/DISCOVERY.md) and [CLI descriptor schema](https://github.com/mapherez/nox-mcp/blob/master/contracts/cli.schema.json) define the protocol boundary. Product-specific operations belong in the respective product documentation.
