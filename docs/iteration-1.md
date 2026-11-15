# Iteration 1 review

11 November 2026. Evidence from the local Windows preview review.

## Implemented

- Angular login, intake, filtered/paginated directory and private item detail.
- Go credential verification, revocable server-side sessions and stored role permissions.
- SQLite persistence, append-only history and idempotency in one transaction.
- Staff holding-point checks and server-side public/private field separation.
- First-run local course account setup. No university authentication integration.
- Windows Wails executable with embedded Angular assets. Browser development workflow retained.

## Evidence

Nine backend tests pass: credentials/roles; private fields; intake validation and holding point; concurrent duplicate requests and changed payload; history-write failure rollback; filters/pagination/final items; restart persistence and lost-response retry; cross-origin writes; setup cannot reset existing accounts.

The Angular production build succeeded. The Windows executable built successfully (16.35 MiB). Native UI verification signed in as reception and read the explicitly labelled fixture previously saved through the browser intake form. The real detail view returned its stored private fields to its creator.

One Windows process-tree sample, PID 11448, seven processes including WebView children: 410.40 MiB summed working sets; 87.09 MiB private working sets; 260.20 MiB private committed memory. Shared pages can be counted repeatedly in the first figure. Reproduce with `scripts/measure-memory.ps1 -AppProcessId <pid>`. Raw local capture: ignored `tmp/memory-native.json`. This is not a sustained load test or cross-framework comparison. Host: Windows 11 Home 10.0.26300, Intel Core i3-1115G4, approximately 7.65 GiB OS-reported physical RAM. WebView version was not recorded for this initial sample.

Window-handle availability was observed after 1,738 ms in one launch. This does not establish UI-ready time or cold-start performance.

## Remaining checks and limits

- Linux build/run, full mobile/browser matrix, keyboard/accessibility audit and M2 load/participant tests are pending.
- Shared remote desktop sessions, HTTPS hosting, deployment and recovery procedures need integration validation. The local executable does not automatically synchronize with other installations.
- Reports, photos, matches, claims, handover and custody changes are not yet implemented. Concurrent approval tests belong to iteration 3; only concurrent intake retry is covered now.
- Q-12 and Q-13 remain unresolved; no approved-claim expiry, cancellation or withdrawal policy was added.
- Runtime npm audit reported no advisories. Development tooling reported 16 moderate transitive advisories; no forced major-version downgrade was applied.
- No numeric RAM budget has been agreed. The initial sample is a baseline, not acceptance of memory usage.

Review this increment before iteration 2. Publishing implementation requires explicit approval for that push.
