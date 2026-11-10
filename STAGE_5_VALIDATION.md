# Stage 5 — Real Backend validation

Status: **partial installed results recorded; no claim of complete validation**.

**Owner update, November 10, 2026:** Yard status execution succeeded through the
CLI and Gateway. After the Backend publication correction, the owner also
confirmed **Apps -> NoX Backend -> Service status**, returning ready status from
Backend **v1.2.2**. The remaining checks below are a record of checks not reported,
not a newly imposed test round. The owner rejected the Yard menu interaction
design and deferred that work. Completion and further scope are owner decisions.

This checks the existing CLI -> Gateway -> Backend journey. It adds no tools,
changes no authorization rules, and does not require a new connection setup.
Run installed checks yourself in your terminal. Do not send API keys, vault
contents, credential-store exports, or personal account details in reports.

## First check: existing read-only menus

Start the CLI using `go run ./cmd/nox` from this repository, or use your existing
source-built executable. Use the Gateway and credential already configured.

1. Open **Apps -> NoX Backend -> Service status**.
2. Press Enter on the tool details to start the interaction. Review it, then
   press Enter to execute. Merely selecting a menu must not run the action.
3. Check that the result is readable and reports the Backend service status.
   Pressing Enter on the result must not run it again. Esc returns to details.
4. Repeat for **Service information** and **Current account**, if your role
   exposes them. Check the identity locally; do not paste personal details.
5. Open **Vaults -> List**, review, and execute. Confirm that the result matches
   your account. Record success/failure without sharing existing vault data.

If Backend is absent, an action is unavailable, or the CLI reports incompatible
metadata, record the visible diagnostic and stop that path. Do not change
Backend/Gateway security settings to bypass it.

Report this first block before proceeding to data-changing checks. A useful
report is: service status passed/failed; information passed/failed; account
correct/incorrect; vault list passed/failed; exact sanitized error, if any.

## Disposable vault lifecycle

These steps create and delete data. Run them only when you choose to begin this
block. Use newly created empty test vaults, never an existing personal vault.
No file upload or real document content is needed.

1. In **Vaults -> Create**, enter a distinctive name such as
   `nox-cli-stage5-<your-test-run>`. Review the name before approving.
2. Create a second empty vault with exactly the same test name. Use **Vaults ->
   List** to record their two different IDs locally as A and B. If duplicate
   names are refused by Backend, record the actual result; do not change its rules.
3. Open **Vaults -> Check status**. The selector must distinguish A and B by ID
   even though their names match. Select A, review, execute, and check the result.
4. Open **Files -> List**, select A, review, and execute. An empty file list is a
   valid result for a new empty vault.
5. Open **Vaults -> Delete**, select A, and inspect the confirmation. Cancel the
   review once with Esc. List again: A must still be active.
6. Repeat Delete for A and approve. Open **Vaults -> Restore**: A must be offered
   as deleted, while B remains active. Restore A and verify the active list.
7. Delete A again. In **Vaults -> Permanently delete**, select A. Enter an
   incorrect confirmation: it must not execute. Then type A's exact vault ID,
   review the target, and approve. A must disappear from both lists.
8. Delete and permanently delete B using B's exact ID. Check that only the two
   newly created test vaults were removed.
9. If there are no other deleted vaults, Restore/Permanently delete must show an
   empty selector and prevent submission. Do not delete unrelated data to force
   an empty list; otherwise record this installed check as not exercised.

Return from each completed action before starting the next. Selectors must load
current active/deleted choices instead of reusing an obsolete selection.

## Key replacement and interrupted connections

These checks affect the running installation or its configured credential.
They remain owner-run checks; they are not executed by the supporting unit tests.

- Rotate your personal key in Backend when ready. The idle CLI need not react.
  After Backend -> Gateway propagation, use **Settings -> Test saved connection**
  or make another real Gateway exchange. Rejection must block further Suite calls
  and request replacement, without printing the old key.
- Enter the new key in the masked Settings field and use **Save and test**.
  Reopen Apps and begin a new interaction. A review created with the old key
  must not remain executable. The implementation's single-slot replacement is
  checked with an in-memory vault; this guide does not require opening Keychain
  or exporting secrets to prove it.
- If you choose to test a Gateway or Backend restart, first use a read-only
  operation. An outage must produce a visible error, and reconnecting must not
  replay an earlier write or skip review. Do not interrupt unrelated work.
- A canceled or interrupted write can have an unknown outcome. Inspect **Vaults
  -> List** before deciding whether to submit anything again; never infer rollback
  from a lost connection.
- Leave a disposable action at review while its target is removed separately,
  then submit it. An obsolete target must fail visibly without selecting another
  vault or automatically replaying the request.

Foreign-account visibility requires a separately authorized test identity and
data. Do not create accounts or weaken access controls for this check. If no such
identity is available, record installed isolation acceptance as not exercised;
local permission tests are supporting evidence, not a substitute.

## Acceptance record

Keep this record free of secrets and personal data:

| Check | Result / evidence |
| --- | --- |
| CLI commit/build | Pending owner report |
| Installed Gateway version | v0.0.1 observed from public health response; authenticated discovery not validated in this attempt |
| Installed Backend version | v1.2.2, reported by owner with successful service-status result |
| Discovery and read-only operations | Owner confirmed Yard status and Backend service status; other operations not reported |
| Create, duplicate selectors, status, empty files | Pending |
| Cancel, soft-delete, restore, permanent delete, cleanup | Pending |
| Empty deleted selection | Pending, or explicitly not exercised |
| Foreign-account/role isolation | Pending, or explicitly not exercised |
| Key rotation and replacement | Pending |
| Outage, reconnect, stale review, no mutation replay | Pending |

On November 9, 2026, focused local tests passed in `internal/interaction`,
`internal/tui`, `internal/setup`, and `internal/gateway`. They covered selectors,
review/confirmation, workflow failure, cancellation, stale definitions, rejected
credentials, credential replacement, exact JSON results, and no automatic replay.
They used in-memory test credentials and disposable loopback MCP servers. They
neither read the personal credential slot nor contacted the installed Suite.

The earlier agent-run attempt stopped at secure credential access before any
successful authenticated discovery or Backend tool call. It is not the latest
installed result: the owner subsequently confirmed the two status operations
recorded above. No personal credential access is needed for this documentation
update. Stage 6 has not started; no additional test round is introduced here.
