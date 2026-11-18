# Iteration 1 review

This is my record of the local Windows preview review on 11 November 2026

I’m keeping it as the original snapshot, with current progress in the [test map](test-map.md)

## What I had working

- Angular login, intake, filtered/paginated directory and private item detail
- Go credential verification, revocable server-side sessions and stored role permissions
- SQLite persistence, append-only history and idempotency in one transaction
- Staff holding-point checks and server-side public/private field separation
- First-run setup for local course accounts
- Windows Wails executable with embedded Angular assets, with the browser development workflow kept

## What I checked

I had nine passing backend tests covering credentials/roles, private fields, intake validation and holding point, concurrent duplicate requests and changed payload, history-write failure rollback, filters/pagination/final items, restart persistence and lost-response retry, cross-origin writes, setup cannot reset existing accounts

I built Angular and the 16.35 MiB Windows EXE, then signed in as reception in the native app and opened the labelled fixture saved from the browser intake form

The creator could read the stored private fields

I sampled PID 11448 and its WebView children, seven processes in total, and got 410.40 MiB summed working sets, 87.09 MiB private working sets and 260.20 MiB private committed memory

The summed working sets can count shared pages more than once, so I still needed sustained measurements before drawing conclusions about memory

The machine was running Windows 11 Home 10.0.26300 on an Intel Core i3-1115G4 with about 7.65 GiB OS-reported physical RAM, and I hadn’t recorded the WebView version for this sample

The raw capture is in the ignored `tmp/memory-native.json`, and the command to take another sample is `scripts/measure-memory.ps1 -AppProcessId <pid>`

I saw a window handle after 1,738 ms in one launch, with UI-ready and cold-start timing still left to measure

## What was left at that review

- Linux build/run, full mobile/browser matrix, keyboard/accessibility audit and M2 load/participant tests are pending
- Shared remote desktop sessions, HTTPS hosting, deployment and recovery procedures need integration validation, and the local executable does not automatically synchronize with other installations
- Reports, photos, matches, claims, handover and custody changes are not yet implemented, with concurrent approval tests planned for iteration 3 and intake retry checked at this point
- Q-12 and Q-13 remain unresolved, no approved-claim expiry, cancellation or withdrawal policy was added
- Runtime npm audit reported no advisories, while development tooling reported 16 moderate transitive advisories, no forced major-version downgrade was applied
- No numeric RAM budget has been agreed, so the initial sample is a baseline, not acceptance of memory usage

I wanted this increment reviewed before iteration 2, with my approval required before pushing
