# Test map and production gates

The original nine automated tests cover the Windows preview workflows. Stage 0 adds backend coverage for found-item photos, trusted proxy identity, and snapshot recovery. Collect structured pass/fail results using the [feedback form](FEEDBACK.md) as testing continues.

The iteration column below refers to the original preview plan. Use the [M2 requirements register](m2-requirements.md) for the current stage assignment and the [diagram checks](milestone-coverage.md) for M3 evidence.

| Area | Evidence now | Still required | Iteration |
| --- | --- | --- | --- |
| Credentials and roles | Backend checks cover test login and opt-in campus self-signup, including role and password storage checks. Windows sign-in observed in the original preview. | Deployed HTTPS signup and login, native remote cookies, abuse tests and separate provider decision for Google sign-in. Retain simulated accounts for CON-002. | 0 / 1 / 5 |
| Intake and directory | Validation, holding-point and pagination tests pass. Browser save and native read observed. | Friend test results, date/length boundaries, keyboard errors, full mobile matrix | 1 / 5 |
| Private fields | Backend access tests pass | Each role on deployed service, attachments and exports when introduced | 1–5 |
| Atomic save and retry | Concurrent duplicate, changed payload, history rollback and restart retry tests pass | Network interruption, disk-full/locked store, process termination and multi-client deployed tests | 1 / 5 |
| Reports and photos | Found-item photo API tests cover the exact byte boundary, invalid/duplicate files, private reads, retry and transaction rollback | Azure upload through HTTPS, high-resolution phone images within the input cap, interrupted uploads, photo UI and lost-report photos. Review the 32 megapixel decode guard against DR-005 before release. | 0 / 2 |
| Matching and claims | Not implemented | Deterministic suggestions, real claim state, duplicate protection, claimant visibility | 2 |
| Verification and handover | Not implemented | Competing approvals, verification checks, rejection/review, authorization, repeat handover and rollback | 3 |
| Custody and dispositions | Not implemented | Transfers, contact attempts, permitted terminal states, atomic history; resolve Q-12/Q-13 before affected behavior | 4 |
| Backup and restore | Local snapshot reopened with the same photo bytes, records, history and retry data. Systemd timer prepared for the first and third Sundays at 02:00 Dubai time. | Validate timer and first scheduled snapshot on Azure. Verify an off-VM copy, restore under the full M2 fixture, restored access checks and recovery timing. | 0 / 5 |
| Directory withdrawal | Backend tests cover FR-017 removal from search, admin access, retained photo/history, retry and rollback. No permanent business-record delete route. | Add the admin control and integrate with claim and match eligibility. Approved-claim handling remains open under M3 Q-13. | 0 / 1 |
| Our hosted service | Azure staging VM exists. Hardened units, Caddy configuration and a checked installer are prepared locally. | Install the reviewed build; verify HTTPS, persistent disk, authenticated remote sessions, independent clients sharing records, runtime memory, monitoring and restart/recovery | 0 / 5 |
| Windows release | Local executable builds and runs | Clean machine, installer/update behavior, signature/distribution decision, WebView prerequisite and upgrade checks | 5 |
| Linux release | Not built | Build/package, install, native WebKit behavior, keyboard, storage paths and supported distributions | 5 |
| Browser/mobile/accessibility | Responsive layout implemented | 360px workflow, supported browser matrix, keyboard-only and screen-reader audit, focus/error handling and contrast | 5 |
| Performance | One Windows process-tree memory sample | Cold/warm UI-ready start; sustained whole-tree RAM/CPU; agreed budget; M2 RD/RL load and storage targets | 5 |
| Usability | Guide prepared | Friend feedback; M2 participant protocol, 8/10 search-to-claim within 3 minutes, median intake at most 60 seconds | 5 |
| Dependencies and release security | Runtime npm audit clean in prior check | Resolve/review development advisories; Go dependency scan, threat review, secret/log checks and release recheck | 5 |

## M2 measurements still open

Use M2 section 3.3.1's full RD/RL definitions. Selected targets: 20 concurrent signed-in users; 2,000 found items; 2,000 lost reports; 1,000 claims; 200 stored 1 MB photos; 95% of report/claim submissions within 3 seconds under the specified conditions. Do not replace the baseline with an easier local benchmark. Fixture datasets must remain labelled test data.

## Production decision

Production-ready remains the required destination, not the status of this preview. All implemented business workflows must pass their authorization, atomicity, failure and deployment checks. Windows and Linux evidence, backup restoration, shared-server behavior and baseline acceptance evidence are release gates. Open policy questions must be resolved or affected operations explicitly withheld. No invented deployment status, uptime or performance claims.

RIT hosting is unavailable. The shared service targets the Azure for Students staging VM. Deployment evidence for this release increment is pending. Check Azure costs, persistence and recovery against the baseline before the course release.
