# Test map and production gates

The original nine automated tests cover the Windows preview workflows. Stage 0 adds backend coverage for found-item photos, trusted proxy identity, and snapshot recovery. Collect structured pass/fail results using the [feedback form](FEEDBACK.md) as testing continues.

The iteration column below refers to the original preview plan. Use the [M2 requirements register](m2-requirements.md) for the current stage assignment and the [diagram checks](milestone-coverage.md) for M3 evidence.

| Area | Evidence now | Still required | Iteration |
| --- | --- | --- | --- |
| Credentials and roles | Backend checks cover test login and opt-in campus self-signup, including role and password storage checks. Windows sign-in observed in the original preview. The restored local API passed campus signup, sign-in, cookie flag checks and logout revocation. The rebuilt native Windows app returned a Secure, HttpOnly, SameSite=Strict cookie issued by a loopback test endpoint and loaded config and directory data through its shared-service proxy. | Deployed HTTPS signup and native login with a real staging account, abuse tests and separate provider decision for Google sign-in. Retain simulated accounts for CON-002. | 0 / 1 / 5 |
| Intake and directory | Validation, holding-point and pagination tests pass. Browser save and native read observed. | Friend test results, date/length boundaries, keyboard errors, full mobile matrix | 1 / 5 |
| Private fields | Backend access tests pass | Each role on deployed service, attachments and exports when introduced | 1–5 |
| Atomic save and retry | Concurrent duplicate, changed payload and history rollback tests pass. A separate SQLite writer lock and a SQLite page-capacity limit each caused a local photo save to return 503 with no partial data. A local HTTP upload cut off mid-photo wrote nothing. After the successful retry and a forced process restart, the same key replayed the saved record and its photo remained readable. | Verify multi-client retry and restart on Azure. The page-limit test exercises SQLite's full condition without filling the VM filesystem. | 1 / 5 |
| Reports and photos | Found-item photo API tests cover the exact byte boundary, invalid/duplicate files, private reads, retry, transaction rollback, overlapping uploads and safe retry at the upload limit. Chrome against the Windows executable in local server mode verified the preview, same-key retry after a dropped response, one saved record, creator and admin access, and a campus user's denied photo request. Local standalone API accepted a 48 MP, 7.41 MB baseline 4:2:0 JPEG and stored 0.39 MB. | Repeat in the native Windows client against Azure, implement lost-report photos, and resolve DR-005 coverage for photos within the byte limit but above the profile-specific pixel limits. | 0 / 2 |
| Matching and claims | Not implemented | Deterministic suggestions, real claim state, duplicate protection, claimant visibility | 2 |
| Verification and handover | Not implemented | Competing approvals, verification checks, rejection/review, authorization, repeat handover and rollback | 3 |
| Custody and dispositions | Not implemented | Transfers, contact attempts, permitted terminal states, atomic history; resolve Q-12/Q-13 before affected behavior | 4 |
| Backup and restore | The backup command opens the source read-only and rejects missing, empty or unrelated database files. A separate local API opened a checksum-matched snapshot containing 2,001 found items, 2,001 history entries and 200 photos of 1 MB each. Integrity and foreign-key checks passed, photo access stayed private, and replay returned the same saved record. Systemd timer is configured for the first and third Sundays at 02:00 Dubai time. | Validate the timer and first scheduled snapshot on Azure. Restore an off-VM copy with the full M2 fixture and record access checks and recovery timing. | 0 / 5 |
| Directory withdrawal | Backend tests cover FR-017 removal from search, admin access, retained photo/history, retry and rollback. No permanent business-record delete route. | Add the admin control and integrate with claim and match eligibility. Approved-claim handling remains open under M3 Q-13. | 0 / 1 |
| Our hosted service | On 2026-11-16 the public Azure staging setup endpoint responded over HTTPS with signup enabled. HTTP redirected to HTTPS, and an unauthenticated session request returned 401. The updated standalone API cross-compiled to Linux amd64. Local deployment fixtures passed: healthy binary replacement, failed health check restoring the previous binary, overlapping deployment refusal and bad checksum refusal. | Run the binary update on Azure and verify persistent disk, authenticated remote sessions, independent clients sharing records, runtime memory, monitoring and restart/recovery. | 0 / 5 |
| Windows release | Local executable builds and runs | Clean machine, installer/update behavior, signature/distribution decision, WebView prerequisite and upgrade checks | 5 |
| Linux release | Not built | Build/package, install, native WebKit behavior, keyboard, storage paths and supported distributions | 5 |
| Browser/mobile/accessibility | Responsive layout implemented | 360px workflow, supported browser matrix, keyboard-only and screen-reader audit, focus/error handling and contrast | 5 |
| Performance | One Windows process-tree memory sample | Cold/warm UI-ready start; sustained whole-tree RAM/CPU; agreed budget; M2 RD/RL load and storage targets | 5 |
| Usability | Guide prepared | Friend feedback; M2 participant protocol, 8/10 search-to-claim within 3 minutes, median intake at most 60 seconds | 5 |
| Dependencies and release security | Runtime npm audit clean in prior check | Resolve/review development advisories; Go dependency scan, threat review, secret/log checks and release recheck | 5 |

## M2 measurements still open

Use M2 section 3.3.1's full RD/RL definitions. Selected targets: 20 concurrent signed-in users; 2,000 found items; 2,000 lost reports; 1,000 claims; 200 stored 1 MB photos; 95% of report/claim submissions within 3 seconds under the specified conditions. Do not replace the baseline with an easier local benchmark. Fixture datasets must remain labelled test data.

In one Windows loopback run with the standalone API and `GOMEMLIMIT=384MiB`, the 48 MP fixture took 0.687 s to save and 0.017 s to replay. Sampled server working set rose from 28.7 MiB after login to a peak of 135.6 MiB during upload. This sample excludes Azure network latency, concurrent users and the desktop process tree.

A separate loopback run started with 500 found items and 100 stored 1 MB photos. Twenty simulated staff sessions completed 200 HTTP requests without errors. The sample covered 140 searches (maximum 28 ms), 20 record views (24 ms), 24 photo-free saves (40 ms) and 16 saves with 1 MB uploads (358 ms). Record, photo and history counts included every save. Full NFR-001 to NFR-004 acceptance still requires the remaining record types, the deployed client and three reference-load runs.

During a local Windows backup rehearsal, the 200.84 MB snapshot took 2.36 s. The live API completed 100 found-item saves before the backup process exited, with a slowest save of 93 ms. After copying the snapshot into a separate data directory, the restored standalone API reported ready in 0.53 s. The restore timing starts after the copy, and the fixture covers the found-item and photo portion of M2.

## Production decision

Production-ready remains the required destination, not the status of this preview. All implemented business workflows must pass their authorization, atomicity, failure and deployment checks. Windows and Linux evidence, backup restoration, shared-server behavior and baseline acceptance evidence are release gates. Open policy questions must be resolved or affected operations explicitly withheld. No invented deployment status, uptime or performance claims.

RIT hosting is unavailable. The Azure for Students staging VM serves the public API. Check Azure costs, persistence and recovery against the baseline before the course release.
