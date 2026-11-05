# NoX CLI — Implementation Plan

Date: November 4, 2026

Status: Stages 1–3 were owner-accepted, including real Gateway discovery (38 tools), menu navigation and the final visual refinement. Stage 4 — Discovery and execution is implemented and locally verified, pending owner review. Stages 5–6 have not started. Remote CI, CLI releases and hosted installer publication remain pending.

## 1. Purpose, scope, and source of decisions

Implement the human terminal interface for NoX Suite. This plan belongs to `nox-cli` and develops **11. Implementar a CLI sobre esse percurso** in the fixed Suite correction sequence. Starting CLI work before adapting the remaining products is the owner's explicit change of work order: validate the actual experience with the prepared Backend before continuing to Yard, Bot, or other products.

This is one application scope delivered through six checkpoints, not a temporary product contract. Stop at the end of each numbered stage, report what changed and what was verified, and wait for the owner's OK before the next stage. Keep the stage names and numbers below stable.

References:

- [General Suite plan, section 11](https://github.com/mapherez/nox-mcp/blob/master/Documents/NOX_SUITE_IMPLEMENTATION_PLAN.md#11-nox-cli).
- [Gateway contract](https://github.com/mapherez/nox-mcp/blob/master/Documents/GATEWAY_CONTRACT.md), particularly aggregation, auxiliary references, discovery, authorization, and execution.
- [Discovery and CLI descriptors](https://github.com/mapherez/nox-mcp/blob/v0.7.3/Documents/DISCOVERY.md).
- [Authoritative CLI descriptor schema](https://github.com/mapherez/nox-mcp/blob/v0.7.3/contracts/cli.schema.json).
- [Same-host service integration](https://github.com/mapherez/nox-mcp/blob/master/Documents/SUITE_SAME_HOST_INTEGRATION.md).
- [Prepared Backend menu descriptions](https://github.com/mapherez/nox-backend/blob/master/docs/mcp.md#human-menus-through-nox-discovery).

The following owner decisions, made after the general plan, define this CLI's current scope:

1. Every NoX application uses **US English (EN-US)** for interfaces, prompts, validation, errors, and documentation. Portuguese conversation examples are not product copy.
2. The root menu is **Apps / Settings / Exit**. Begin with keyboard-driven menus and execution. The exact NoX character-art header is refined during the interface stage.
3. The CLI can run on another computer, but reaches the Gateway only through a private LAN or private access such as Tailscale/Twingate. Public Gateway addresses/tunnels are unsupported.
4. AI clients connect directly to the Gateway's MCP endpoint using their own client configuration. **This CLI does not implement an MCP server.** Earlier references to a CLI-hosted MCP server do not authorize building one here.
5. Written business commands, an embedded AI agent, chat, NoX Bot chat integration, and external MCP-server aggregation are outside this work. Do not build dormant implementations for them.
6. Key invalidation is detected on the next actual Gateway exchange that reports invalid credentials. **No CLI push-invalidation channel, authentication polling loop, or check on every menu Enter.** This replaces the earlier suggestion of receiving an immediate notification while idle.
7. Use per-user installation scripts, a compiled executable, secure native credential storage, in-app updates, and an uninstall script. Manual archive extraction is not a supported user installation flow.

This file records these decisions without rewriting the original Suite plan or changing shared contracts. Concrete incompatibilities found during integration must be reported and scoped before changing another repository's behavior.

## 2. Current state and integration boundary

At plan creation:

- `nox-cli` contains only its initialized Git repository. There is no existing CLI runtime, installer, release, or terminal renderer.
- NoX MCP **0.7.3** is released. Its standard `tools/list` excludes reserved `_meta.cli`; canonical `nox/tools/definitions` retains paths and separate optional descriptors.
- Backend source consumes SDK 0.7.3 and has descriptors for its ten existing tools: navigation, active/deleted vault selectors, and human confirmations. That source preparation does not prove those descriptors are installed in the running environment.
- Gateway already implements authenticated MCP aggregation and NoX discovery. Its dependency, published image, and preservation of the prepared descriptors must be checked during interoperability work; SDK, source, and installed-product versions must not be treated as interchangeable.
- CLI, Gateway, and SDK tests do not certify the complete installed Suite journey.

Runtime path:

```text
Human -> nox -> private Gateway /mcp -> authorized Suite product
AI client ----> private Gateway /mcp -> authorized Suite product
```

The CLI never calls a Suite product, Backend storage, Docker, or the private service socket directly. It does not enroll products, issue keys, manage Google login, duplicate the registry, or acquire a service credential. Yard retains installation/lifecycle responsibilities. The same-host rule applies between Suite services, not between the user's CLI computer and Gateway.

Product IDs, display names, tool names, aliases, paths, and schemas are received data. Backend is the real acceptance target, not a special case in production CLI code. New conforming products must appear without rebuilding CLI menus.

## 3. Implementation foundation

Use **Go**, the official MCP Go client, NoX MCP **0.7.3**, and the **Bubble Tea v2 / Bubbles v2 / Lip Gloss v2** terminal UI libraries. Select and lock compatible concrete dependency versions during stage 1; do not require end users to install these build tools.

The installed command is `nox` (`nox.exe` on Windows). The application is one native process with a persistent interactive session, asynchronous network work, and no mandatory Docker container, browser UI, background daemon, or additional listener.

Keep these responsibilities separate in implementation, without generating empty package scaffolding:

- Terminal state, navigation, forms, and result views.
- Gateway transport, authentication failures, protocol negotiation, and discovery cache.
- Generic descriptor interpretation and interaction execution.
- Non-secret configuration and native credential-store adapters.
- Installation ownership, release metadata, updates, and removal.

Target Windows, macOS, and Linux; plan release artifacts for x86-64 and ARM64. Verify dependency/platform support and build/runtime checks rather than claiming support from a cross-compilation alone. Unsupported environments need a clear diagnostic, not an unrelated fallback implementation.

## 4. Terminal experience

The initial screen has NoX branding in the upper-left, connection/version status, a content area, and contextual keyboard help. The exact character-art logo is a presentation checkpoint, not an extra graphics subsystem.

```text
NoX                                           Connection status

  Apps
  Settings
  Exit

Arrow keys: Move    Enter: Select    Esc: Back
```

- Apps opens the products available to the current credential, then each product's published menu tree. Product labels come from Gateway presentation; identity comes from product IDs and verified origin metadata.
- Settings contains Gateway connection, API-key replacement/removal, version/update information, and documented installation/removal information. Its wording is EN-US.
- Exit restores terminal state and ends the session cleanly. Interruptions must also restore the terminal.
- Support selection, back navigation, breadcrumbs, menu search, scrolling, terminal resizing, and readable empty/loading/error states. Basic use must not require a mouse.
- Esc goes back or cancels the current interaction; Ctrl+C behavior must distinguish an executing operation from exiting the application. Never silently repeat a mutation after cancellation.
- Network calls must not block keyboard handling. Preserve valid form state while editing; discard stale authorization/review state when required.
- Unconfigured, disconnected, credential-rejected, unsupported, empty, and temporarily unavailable are distinct states. Keep Settings and Exit available when Suite actions are blocked.
- Do not add a chat box, command interpreter, model selector, or product-specific dashboard to this scope. Keep interaction logic separate from rendering so later work does not require replacing the core.

Use `structuredContent`, output schemas, ordinary MCP content, and `isError` for a generic scrollable result/error view. Preserve precision, pagination information supplied by tools, and ordinary content semantics. Do not invent a result-rendering extension or treat tool content as executable terminal instructions. Escape unsafe terminal control sequences in untrusted labels/content; never execute strings as shell commands.

## 5. Gateway configuration, private access, and credentials

### 5.1 Connection and network policy

The user supplies a Gateway address and Backend-issued personal API key in Settings. Derive the existing `/mcp` endpoint from an origin without application prefixes, following the existing URL contract. No product addresses, homelab IPs, DNS suffixes, certificate paths, or network-specific values are embedded in the application.

- Allow supported private access by IP or hostname, including LAN, loopback for local use, and private Tailscale/Twingate routes.
- Do not require the CLI and Gateway to have the same subnet, run on the same computer, or share a DNS suffix.
- Resolve and check the actual connection destination, not just the spelling of a hostname. Account for VPN-assigned address ranges, IPv6, and IPv4-mapped IPv6; do not classify all VPN addresses as public simply because they are outside RFC 1918.
- Reject public Gateway destinations. Do not follow a redirect, proxy setting, or alternate resolved address that bypasses the private-target rule or leaks the credential. Include LAN/VPN and public-destination cases in tests.
- A private IP is not proof of identity or a firewall. Keep personal-key authentication and the Gateway's own access controls. CLI validation does not claim to make a publicly exposed Gateway private.
- HTTP is supported for the agreed private access. HTTPS is optional and retains normal certificate verification; no insecure TLS fallback, mandatory certificate setup, or certificate copying.
- Public HTTPS access to the official installer/release service is separate from the private-only Gateway rule. Never send the Gateway API key to release/update hosts.

Stage 2 must turn these rules into an explicit tested destination policy, using ordinary defaults rather than requiring users to supply CIDRs or customize their installation network.

### 5.2 Secret storage and replacement

Use Windows Credential Manager, macOS Keychain, and an appropriate encrypted Linux Secret Service backend. The non-secret configuration contains the Gateway origin and a secret reference, never the API key or decryption material. Do not silently fall back to plaintext storage. When secure storage is unavailable, explain the limitation and offer explicit session-only use without persistence.

Use one stable credential identity: **`nox-cli/api-key`**, mapped to the native store's service/account or target fields. Replacing a key is an upsert of that identity, not a new suffixed entry. There is one active Gateway configuration and one associated personal key in this scope; do not introduce multi-profile credential management.

- Input is masked, supports paste, and does not enter shell history, logs, telemetry, crash diagnostics, or error bodies. Do not require the key as a command-line argument.
- Validate the current Backend key format before contacting Gateway. Current generation is `noxsync_` plus a 32-byte base64url value: **51 characters in total**. This is compatibility with current generation, not authorization and not a new shared contract for all future key formats.
- Report useful EN-US errors such as `Invalid API key length. Expected 51 characters.` and `Invalid API key format.` without echoing the value.
- Validate the candidate with Gateway before persisting it. A failed connection is not proof of an invalid key and must not overwrite an existing valid credential.
- Once saved, display only `API key configured`, with Replace and Remove actions. No reveal, copy, export, or old-key fallback.
- On successful replacement, clear credential-bound catalogs, forms/reviews, authorization state, and previous connection state before loading the new authorized view.
- Credentials must be recoverable by the process for outgoing authentication; minimizing their lifetime in memory is not a claim that they are hashed or irrecoverable. Never forward them to products, and never store them in menu arguments.

### 5.3 Invalidation and long working sessions

Ordinary local navigation, opening cached submenus, typing, and review do not require a Gateway request. Network exchanges occur when connecting, revalidating discovery, fetching dynamic options/previews, or executing a tool/workflow step. Do not add a separate authorization request to each keystroke or Enter.

Backend remains the authority. Its key-rotation invalidation already reaches Gateway through the Suite event mechanism; existing delegation/cache rules remain server responsibilities. The CLI blocks Suite operations **when a Gateway exchange reports the credential is invalid**, clears reusable credential-bound state, and prompts for replacement. It does not claim to know about a rotation while idle or before server-side propagation has occurred.

Do not treat a tool-specific `FORBIDDEN`, domain error, timeout, unavailable product, or Gateway outage as proof that the whole API key is invalid. Network errors preserve the stored credential while preventing unverified execution. Authentication failures must not be bypassed by a stale local cache.

A working session may last hours. Existing server authorization renewal is distinct from that session, and human time spent in a form does not start an RPC deadline. Define cancellation, per-call budgets, and unknown mutation outcomes without requiring another Google login. Do not replay a write automatically after reconnecting or replacing a key.

## 6. Generic discovery, menus, and execution

Negotiate the Gateway's actual MCP and `nox/discovery` support using existing SDK mechanisms. Use its supported Streamable HTTP JSON exchange; do not make the CLI depend on currently unsupported Go MCP SSE behavior. Load and validate authorized catalogs and canonical definitions; keep private caches partitioned by the configured origin and credential context. Begin with an in-memory cache, not a new persistent tool database.

- Interpret `_meta.cli` paths and the separate versioned `cli` descriptor. Standard `tools/list` is not a source from which to reconstruct missing human descriptors.
- Read Gateway product presentation and `nox/gateway` origin metadata; do not parse an alias string to infer a product, construct aliases, or fetch product URLs directly.
- Support nodes that have both an action and children. Honor navigation labels, groups, and order without adding duplicate hierarchy levels; test the actual received Backend tree.
- Disable ambiguous paths and advanced actions whose required features or auxiliary references are unsupported/missing/unauthorized. Show a diagnostic without hiding unrelated products or guessing an action.
- Resolve options, previews, and workflow references within the originating product using Gateway-rewritten references. Never select a same-named tool from another product or increase permissions to fill a menu.
- Build arguments from the existing input schema and supported descriptor controls: text, password, select, multiselect, checkbox, number, and textarea. Preserve JSON types and required/optional distinctions. Do not insert schema defaults automatically or coerce an empty value into an unrelated type.
- Respect RFC 6901 pointers and binding semantics, required descriptor features, sequential workflow step dependencies, and result references. Read auxiliary data from structured results; do not parse arbitrary text into authorization or workflow bindings.
- Treat an absent optional options array as no choices where appropriate. Keep option IDs separate from labels; distinguish duplicate labels. An empty required selector cannot execute a mutation.
- Tie confirmation to the reviewed arguments, definition fingerprint, and any preview. Edits, changed definitions, or changed previews invalidate prior confirmation. Bind a confirmation value only when the descriptor requests it; never add a mandatory tool argument or invent a preview endpoint.
- Supply the existing discovery fingerprint guard on execution where supported. On stale catalog/definition or removed access, refresh and require another selection/review rather than executing a different action or automatically replaying a write.
- Surface real MCP errors/results, progress when supported, unavailable states, deadline/cancellation results, and unknown outcomes. Refresh affected choices after successful mutations; do not display a success before confirmation from the server.

Human confirmation is an interface safeguard, not a replacement for server authorization. Products keep their schemas, tool identities, domain rules, and execution behavior.

## 7. Installation, update, and uninstall ownership

### 7.1 Per-user layout and supported entry points

| System | Managed installation root |
| --- | --- |
| Windows | `%LOCALAPPDATA%\nox-cli` |
| macOS | `~/Library/Application Support/nox-cli` |
| Linux | `${XDG_DATA_HOME:-$HOME/.local/share}/nox-cli` |

The executable lives directly under that root. Non-secret configuration and a small installation-ownership record also live there. **`install/` beneath the same root is the only managed area for downloads, extraction, verification, update preparation, and necessary short-lived helpers.** Remove staging contents after success, failure, or cancellation. On the next invocation recover/clean an interrupted owned transaction; a power loss cannot run cleanup at the instant it occurs. Do not accumulate old executables or downloaded archives.

Use the `nox-cli` name consistently for directories, PATH markers, native credential ownership, and installation metadata. The secret itself lives in the native vault, outside this directory.

Intended script endpoints, which are not deployed at plan creation:

- `https://cli.mapherez.com/install.sh` and `/install.ps1`.
- `https://cli.mapherez.com/uninstall.sh` and `/uninstall.ps1`.

The scripts select the official published executable for the OS/architecture, verify release checksums, install per user without elevation, and configure command discovery. Users need no Go, Node, Rust, Docker, source checkout, or manual archive extraction. Archives are installer inputs, not another documented installation method.

PATH changes must be idempotent and owned. Windows uses the user PATH; macOS/Linux use clearly marked, minimal additions for supported shells. Do not overwrite unrelated PATH entries, claim an unrelated existing `nox` command, or duplicate profile blocks on reinstall. Explain when a new terminal is needed. Detect unsupported shell/environment cases instead of claiming all terminals were configured.

### 7.2 Updates and removal

- Check official release metadata asynchronously and with bounded frequency/timeouts. Show an update indicator without blocking startup or offline/local use. Do not send credentials or perform silent updates.
- Settings -> Update shows the target version and applies a user-requested, verified release. Use a staged transaction in `install/`, preserve configuration/native credential identity, and leave the old executable usable if replacement fails.
- Handle Windows executable locking using a short-lived owned helper when necessary; no installed update service. Complete cleanup after the update handoff and recover an interrupted transaction on the next run.
- Do not replace the running executable while a business mutation is active. Do not resume/replay that operation after restart. Restart or clearly explain the required restart after a successful update.
- Uninstall scripts must work without a functioning installed executable. Remove only this installation's owned executable/metadata/staging and owned PATH changes. Do not recursively remove a shared directory or follow a changed path/symlink into unrelated data.
- Offer explicit removal of configuration and the owned native credential entry. Keep and full-clean behavior must be clear; do not silently erase unrelated files, other credentials, or shared shell setup.
- Document install, update, uninstall, supported paths/shells, private Gateway setup, secret handling, and interruption recovery in the project's README before release.

## 8. Numbered implementation stages and completion gates

These six stages are internal checkpoints of Suite point 11. They do not rename or renumber the general Suite sequence. The dated implementation record below distinguishes actual work from the plan; writing this document alone did not start an implementation stage.

### Stage 1 — Project foundation

Create the minimal Go project, command entry point, EN-US documentation, selected/locked dependencies, build/version information, and CI foundation. Establish terminal/network/configuration boundaries without implementing future MCP-server or AI modes.

Completion gate:

- The executable starts/exits cleanly and reports its version; normal interruption restores the terminal.
- Dependency/module checks and relevant Go checks pass. Establish platform build checks and identify which platforms have actual runtime evidence.
- README explains the current source-only state. No installer URL, release, feature, or remote workflow is presented as already available.

**Stop for owner review.**

### Stage 2 — Installation and configuration

Implement per-user installation/ownership foundations, shell/PATH handling, Settings connection/configuration, native credential adapters, current key-format validation, private-target enforcement, and a minimal real Gateway connection/discovery check. Script tests may use controlled release artifacts; they must not claim those artifacts are a published application.

Completion gate:

- Configuring a reachable private Gateway authenticates using the masked key and confirms actual negotiated capabilities, including a valid empty catalog.
- Replacing/removing a key uses one stable vault entry. Malformed input, rejected credentials, unavailable vault, session-only use, connection errors, and DNS/redirect/proxy destination bypasses are covered.
- Install/reinstall/error cleanup and PATH ownership are exercised. No plaintext secret or arbitrary installation residue remains.
- Public Gateway destinations are rejected; normal LAN and supported VPN access are not accidentally blocked. HTTPS verification remains enabled when used.

**Stop for owner review.**

### Stage 3 — Interactive interface

Build the actual persistent **Apps / Settings / Exit** interface, NoX header, product navigation, breadcrumbs, search, back behavior, responsive layout, contextual help, and loading/error/empty states. Agree the exact logo/layout at this checkpoint, without delaying the project foundation for artwork.

Completion gate:

- Navigate real Gateway-discovered products/paths by keyboard without hardcoded product menus.
- Resizing, long labels, narrow terminals, empty catalogs, unconfigured/offline state, and clean exit behave predictably.
- Navigating cached menus does not trigger authentication requests on each Enter.
- Owner can review the actual terminal experience; screenshots or fixtures are not evidence of a working network connection.

**Stop for owner review.**

### Stage 4 — Discovery and execution

Complete generic schema-based forms, descriptor controls, dynamic selectors, previews, confirmations, workflows, results, cache/fingerprint handling, deadlines, and cancellation through the Gateway.

Completion gate:

- All descriptor mechanisms required by the published CLI contract are implemented or explicitly block the affected path; unsupported behavior is never silently skipped.
- Tests cover argument dependencies, empty/duplicate options, stale definitions, lost access, failed workflow steps, canceled reads/writes, unknown outcomes, sanitized failures, and mixed authorized/unavailable products.
- Tool calls preserve canonical arguments and current authorization, with no invented business tools, cross-product auxiliary fallback, or automatic mutation replay.
- Product-less fixtures are limited to focused automated interaction/contract tests. They do not become shipped products, menus, or fabricated Suite contracts.

**Stop for owner review.**

### Stage 5 — Real Backend validation

Validate the actual terminal -> Gateway -> Backend path before adapting Yard, Bot, or other products. Install a Backend release containing the prepared EN-US descriptors and use its existing Suite enrollment/availability configuration. Check the installed Gateway's compatibility with canonical discovery and SDK 0.7.3 publication behavior; report specific incompatibilities before proposing separately scoped consumer fixes.

Completion gate:

- Open Apps, discover Backend, and reach its menus through Gateway without entering Backend-specific connection details in CLI.
- Execute the account/status/info operations permitted by the current role. Create/list/select vaults, inspect status/files, soft-delete, restore, and permanently delete disposable owner-approved data through the real menus.
- Verify duplicate names retain distinct IDs, active/deleted selections refresh correctly, empty deleted lists are handled, and foreign data/unauthorized actions remain inaccessible.
- Rotate the personal key in Backend. After normal Backend -> Gateway invalidation propagation, a rejected subsequent Gateway exchange blocks CLI Suite actions and asks for replacement. Saving the new key updates the same vault identity and reloads the authorized view. There is no claim of instant idle notification.
- Reconnection, Gateway/product restart or outage, a stale form, and key replacement do not replay mutations or bypass a review.
- Record versions, actual observations, local/CI checks, and installed manual acceptance separately. Backend remains the acceptance product; no production code branches on its product/tool names.

**Stop for owner review.**

### Stage 6 — Update and uninstall

Finish the Settings update flow, native replacement/helper behavior, uninstall scripts, release artifacts/checksums, hosted installer delivery, README, and platform acceptance. Complete the installation work introduced in stage 2 against actual release packaging.

Completion gate:

- Install, reinstall, update, failed/interrupted-update recovery, uninstall, and explicit full cleanup are validated on supported platforms/shells with clear evidence.
- Validate spaces in user paths, no administrator access, existing-command conflicts, modified PATH blocks, and configuration/credential preservation or explicit deletion.
- Only owned staging/installation/PATH/vault items are changed. Temporary artifacts and update helpers are cleaned; a failed update leaves a usable previous installation.
- Build/release automation verifies the selected commit and artifacts. Document which CI jobs actually run and any platform validation limitation; do not conflate a successful build with installed acceptance.
- Publishing scripts/releases/domain routes and changing installed applications remain explicit owner-controlled actions. Completion of local source work is not permission to publish or deploy.

**Stop for owner review.**

## 9. Completion boundary

The planned menu CLI is complete when a supported script-installed executable can privately connect using its securely stored credential, discover and navigate conforming product menus, execute the published interactions correctly, present errors/results, detect credential rejection on a real exchange, and update/uninstall cleanly with the above evidence.

NoX CLI does not need a built-in AI, chat mode, its own MCP server, external MCP aggregation, Portal, or adaptations of every NoX product to meet this boundary. Additional product adapters must be able to reuse the same generic mechanisms afterward.

Implementation findings are recorded as findings, not silent amendments to agreed plans or security rules. Keep changes in the repository/stage being implemented; report a concrete cross-repository dependency when one is found. Do not declare a stage or the complete CLI ready while required work or validation remains outstanding.


## 10. Implementation record — Stage 1, November 4, 2026

**Implemented:** minimal `cmd/nox` entry point; safe help/version/argument handling;
public build identity; Bubble Tea foundation screen with Bubbles keyboard help
and Lip Gloss rendering; clean exit/signal lifecycle; and an outgoing official
MCP client constructor configured for NoX discovery. The client test uses an empty
in-memory protocol peer, not a running Gateway or a shipped Suite product.

No Gateway transport, configuration store, credential persistence, installer,
product menus, update/uninstall code, or incoming MCP listener was introduced.
Only packages with implemented foundation behavior were created. The approved
requirements and six stage names above remain unchanged.

Pinned direct versions: Bubble Tea 2.1.1, Bubbles 2.2.1, Lip Gloss 2.0.6,
Charm terminal detection 0.2.2, NoX MCP 0.7.3, and MCP Go SDK 1.8.0.

Local evidence on macOS ARM64:

- Go 1.27.3: module verification/tidiness, `go vet ./...`, and
  `go test -race -mod=readonly ./...` passed.
- Minimum Go 1.26.0: tests and module-tidiness checks passed.
- The actual compiled executable reported injected version/commit/date correctly;
  help, argument errors, and redirected-input rejection passed.
- Real PTYs: startup, resize, Esc, Ctrl+C, SIGINT, and SIGTERM passed; checks verify
  input-mode restoration, alternate-screen exit, and cursor visibility. The macOS
  kernel can add `PENDIN` when restoring canonical input; a Python-only raw/restore
  round trip confirmed this. Only that kernel-added flag is permitted to differ;
  every other original terminal attribute is still compared.
- All six Windows/macOS/Linux amd64/arm64 command builds passed with CGO disabled.
  This is compile-only evidence for non-native targets.
- GitHub Actions workflow validation with actionlint passed.

CI is configured for native Linux/Windows/macOS checks, Linux race detection,
Linux/macOS PTY checks, and all six target builds. No GitHub run or remote release
is claimed. Native Windows terminal restoration is not exercised by the POSIX
script and still requires ConPTY/manual evidence before claiming that behavior;
Linux/Windows native execution is pending the corresponding CI/manual runs.

**Owner acceptance:** the owner confirmed the version output and foundation
screen, then explicitly authorized Stage 2 — Installation and configuration.


## 11. Implementation record — Stage 2, November 4, 2026

**Implemented in this repository:** Settings with masked key entry and format
feedback; verified candidate save/replacement, saved-connection testing, explicit
session-only mode and key removal; non-secret settings with private per-user paths;
a single origin-bound native vault slot; authenticated, destination-restricted
Gateway HTTP JSON discovery, including a valid empty catalog; and installation
activation, PATH ownership, POSIX/PowerShell download-and-verify scripts.

The minimal root shell exposes Settings and Exit; Apps remains explicitly
unavailable. Full product menus/search/navigation are still Stage 3. No incoming
MCP server, AI/chat mode, direct product access, service enrollment, Backend auth
change, mandatory TLS, public Gateway support, release or hosting configuration
was added. Changes are limited to `nox-cli`.

The new native-vault dependency is `github.com/zalando/go-keyring v0.2.8`.
Its macOS implementation passes secret data over stdin to the system Keychain
utility, not in a process argument. Windows uses Credential Manager and Linux
uses Secret Service; no plaintext file-store fallback is enabled. The one native
service/account pair is `nox-cli` / `api-key`; session-only mode never reuses an
inactive persisted credential. Removing a key also clears its saved reference.

Replacement is validated before persistence and uses the same slot. Failed
settings persistence restores the previous value when possible; the secret's
origin binding prevents a partially committed address change from forwarding a
saved key elsewhere. A previously rejected stored key is not silently retried.
Ordinary navigation does not make authorization requests. Rejection is detected
on an actual Gateway response, not by an idle notification mechanism.

The private destination policy supports RFC 1918 IPv4, private IPv6, loopback and
shared VPN addressing (`100.64.0.0/10`), including IPv4-mapped IPv6 normalization.
It pins resolved private addresses, rejects mixed/public DNS answers, ignores
HTTP proxy environment settings, refuses redirects, and retains HTTPS certificate
verification. Connection/discovery failure messages never echo a key. Labels from
the remote peer are sanitized before terminal output.

Installer activation preserves unrelated profile content, detects foreign command
and unowned-binary conflicts, records ownership across reinstalls, and stages
files under `nox-cli/install/`. Windows preserves the existing Registry PATH value
type; POSIX supports Bash/Zsh. A profile on another filesystem produces a specific
atomic-installation diagnostic and rollback. Scripts recover stale/empty download
locks and remove their own downloads/helpers; unexpected links/archives/checksums
are rejected. Updates/uninstall/release hosting remain Stage 6 work, as planned.

Local evidence on macOS ARM64:

- Go module verification/tidiness, vet, complete race tests, and minimum Go 1.26.0
  tests passed. A subsequent targeted race check also verified actual first-profile
  rollback when a second profile write fails.
- Actual official MCP HTTP exchanges passed: authenticated empty catalog, key
  rejection, non-auth permission/service failures, denied redirects and untrusted
  TLS. DNS/private/VPN cases and origin pinning were also tested. These disposable
  peers do not establish acceptance against the owner's running Gateway.
- Settings/vault coordination tests passed for same-slot replacement, invalid
  candidates, no key in config files, origin changes, unavailable vault/session
  mode, cancellation, persistence rollback, removal and rejected-key reuse.
- The real macOS Keychain test stored/replaced/read/deleted a uniquely named
  disposable test entry. It did not touch the production `nox-cli/api-key` slot;
  cleanup was checked. Native Windows/Linux vault execution remains remote CI work.
- Installer tests passed for actual isolated activation/PATH edits, idempotence,
  ownership, rollback and preserving existing profiles. Eight POSIX script checks
  passed for verified handoff, bad checksum, traversal/link rejection, interruption,
  stale/empty lock recovery and cleanup, using controlled test artifacts.
- The official PowerShell 7.6.9 parser accepted both Windows scripts on macOS.
  The portable parser runtime/archive were removed afterward. This is syntax
  evidence, not Windows execution; native script and isolated Registry tests are
  configured in CI and are not claimed as already run.
- Compiled process and real PTY checks passed, including entering Settings, masked
  input, resize, Esc, Ctrl+C, SIGINT and SIGTERM with terminal restoration.
- All six Windows/macOS/Linux amd64/arm64 builds passed. Windows vet also passed.
  These cross-builds are not native runtime evidence for the other platforms.
- Workflow actionlint and shell-block syntax checks passed. CI additionally
  provisions an isolated encrypted Linux Secret Service for the native vault test;
  it does not install a keyring service on an end-user machine.

**Checkpoint:** stop for owner review and a private Gateway connection check
through Settings. Source implementation and local verification do not mean a
GitHub workflow, published installer, installed Gateway acceptance or Stage 3 has
completed. Do not start Stage 3 — Interactive interface until the owner's OK.


## 12. Implementation record — Stage 3, November 5, 2026

**Owner authorization:** after confirming a successful Gateway connection and
38 discovered tools in Stage 2, the owner explicitly authorized Stage 3.

**Implemented only in `nox-cli`:** persistent Apps / Settings / Exit navigation;
an adaptive NoX character-art header; product menus from Gateway presentation
and canonical origin/path metadata; descriptor labels, ordering and group labels;
breadcrumbs, keyboard selection/back, local scoped search, scrolling, responsive
layout and read-only tool details. `R` refreshes discovery explicitly; `?` opens
scrollable metadata diagnostics. An action can share its node with subcommands.
Missing paths and unsupported/ambiguous descriptors do not hide valid actions.

Fresh discovery verifies the published response schemas, catalog identity and
complete definition fingerprints, with paginated catalogs and batches of at most
64 definitions. Descriptor interpretation happens per action, so an unknown
required interaction disables that action rather than the entire menu tree.
Snapshots remain in memory and are never authorization grants. No ordinary menu
navigation or search makes a Gateway request. Credential rejection on a real
exchange clears menus and prevents reusing the rejected key; a service outage
remains a distinct retryable state. Settings and Exit remain accessible.

Selecting a tool currently opens its description and input summary. No forms,
auxiliary calls, confirmations, workflows, business-tool execution, commands,
chat, MCP listener, updater or new cross-repository behavior was introduced.
Stage 4 — Discovery and execution remains a separate owner-controlled checkpoint.

Local evidence on macOS ARM64:

- Dependency checksums and module tidiness passed. Full `go vet` and race tests
  passed; minimum Go 1.26.0 tests also passed.
- A real official MCP HTTP peer delivered canonical menu metadata and presentation
  without executing a business tool. Catalog tests covered pagination, 64-item
  definition batching, availability and rejection of altered/missing snapshot data.
- Menu tests covered generic identity/path/label/order handling, action nodes with
  subcommands, conflicting/missing metadata and isolated unsupported descriptors.
- Interface tests covered local search/back/refresh, cached navigation without
  further exchanges, key rejection/no reuse, offline/empty/unconfigured states,
  long labels, scrolling, cancellation/stale results and sizes down to one cell.
- The compiled executable passed isolated real PTY startup, masked Settings input,
  resize and Esc/Ctrl+C/SIGINT/SIGTERM restoration checks. These checks now isolate
  user paths and do not load the developer's configuration or credential.
- All six Windows/macOS/Linux amd64/arm64 builds passed. Cross-compilation is not
  native runtime evidence; Windows terminal restoration and remote native CI
  remain separate checks. No personal API key was read by these local checks.

**Checkpoint:** stop for owner review of the actual Apps menus/header on the
running Gateway. The owner's earlier 38-tool discovery proves Stage 2 connectivity,
not installed acceptance of this new interface. No release, push or deployment was
performed. Do not begin Stage 4 — Discovery and execution until the owner's OK.


## 13. Implementation record — Stage 4, November 5, 2026

**Owner acceptance/authorization:** the owner confirmed real menu navigation,
approved the NoX welcome screen and style, and explicitly authorized the next
stage. The six stage names and shared Suite contracts remain unchanged.

**Implemented in `nox-cli`:** schema-driven text/password/number/checkbox/select/
multiselect/textarea and JSON input; optional omission; dependent selectors with
separate typed values and disambiguated labels; RFC 6901 reads/bindings; read-only
previews; review-bound boolean/typed confirmation; sequential workflows with prior
structured-result bindings; guarded calls and progress/cancellation; and scrollable
structured/text/resource results and errors. The interface uses the approved
palette, adaptive header and keyboard navigation.

All calls stay on the configured private Gateway. Auxiliary references must exist
in the originating product/server. Every call has its own fingerprint guard;
execution revalidates the reviewed definition set, while expired snapshots also
refresh before auxiliary calls. Origin/key partitions prevent a form from being
sent under replaced settings. Server auth/policies remain unchanged. Rejected
credentials clear forms/menus and block reuse; tool permission/domain failures do
not invalidate a valid personal key. No operations replay automatically, including
MCP multi-round-trip requests. Cancellation or failure after dispatch can leave a
write outcome unknown and is reported accordingly. Human review has no deadline;
the network-call budget is two minutes. A write expires discovery for subsequent
requests, and a failed workflow cannot run later steps or rewind completed writes.

The existing `github.com/google/jsonschema-go` 0.4.3 dependency is now directly
used for validation, with no external schema loader or automatic default insertion.
Numeric validation uses a native-number copy while arguments/results retain JSON
number lexemes, including integers beyond IEEE-754 exact range. Integers outside
64-bit local validation, unsupported numeric exponents, external/recursive schemas,
parameter controls inside variable arrays, more than 32 object levels and selectors
over 5,000 options fail explicitly. Text inputs have a 1,048,576-character limit.
These are client diagnostics/limits, not changes to the published Suite contract.
Whole arrays/objects and schema unions can use JSON input. Binary content is not
opened automatically. Output-schema mismatches retain the actual response rather
than claiming rollback or guessing structured data from ordinary text.

Local evidence on macOS ARM64:

- Full `go vet ./...` and `go test -race -mod=readonly ./...` passed. Minimum Go
  1.26.0 tests, dependency verification and `go mod tidy -diff` passed. Importing
  Bubbles textarea required the upstream heredoc test-dependency checksums; those
  are included so CI's tidy check remains reproducible.
- Actual MCP HTTP calls passed for guarded dispatch, changed definitions, tool
  permission/domain failures, credential rejection, cancellation and no replay.
  Raw argument/result checks preserved `9007199254740993` exactly and verified
  credential redaction. Disposable servers were explicitly authorized test peers;
  no live Suite tools or personal keys were used.
- Descriptor tests passed for all supported input kinds, dependent/empty/duplicate
  selectors, exact typed values, preview bindings, edited/invalid confirmations,
  sequential workflow bindings/failure/rewind prevention, JSON pointers, missing
  cross-product references, unsupported requirements, output mismatches and
  bounded numeric/recursive-schema handling.
- Interface tests passed for details -> form -> review -> execution -> result,
  masked passwords including echoed results, typed confirmations containing E,
  JSON/multiline input, optional omission, credential/permission/stale handling,
  changed credential partitions and resizing down to one cell.
- The actual compiled process passed isolated PTY startup, masked Settings entry,
  resize, Esc/Ctrl+C/SIGINT/SIGTERM and terminal restoration. All six target
  Windows/macOS/Linux amd64/arm64 builds passed. Non-native builds do not establish
  native runtime behavior; remote CI and Windows terminal acceptance remain
  separate evidence.

**Checkpoint:** stop for owner review. Stage 5 — Real Backend validation has not
started. This stage does not certify the installed Gateway/Backend journey or
implement update/uninstall. No publication, commit, push, release, deployment or
change to another repository was performed.
