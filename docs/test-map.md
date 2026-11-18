# Test map and production gates

This is where I keep what we’ve checked and what still needs work before release

The first preview had nine automated tests, and I added coverage as photos, reports, claims and the shared server landed

Use the [feedback form](FEEDBACK.md) for group results, the [M2 register](m2-requirements.md) for the baseline and the [diagram coverage](milestone-coverage.md) for M3

The iteration column follows our original preview plan

| Area | Evidence now | Still required | Iteration |
| --- | --- | --- | --- |
| Credentials and roles | Backend checks cover test login and opt-in campus self-signup, including role and password storage checks, Windows sign-in observed in the original preview<br>The restored local API passed campus signup, sign-in, cookie flag checks and logout revocation<br>The rebuilt native Windows app returned a Secure, HttpOnly, SameSite=Strict cookie issued by a loopback test endpoint and loaded config and directory data through its shared-service proxy | Deployed HTTPS signup and native login with a real staging account, abuse tests and separate provider decision for Google sign-in, Retain simulated accounts for CON-002 | 0 / 1 / 5 |
| Intake and directory | Validation, holding-point and pagination tests pass, Browser save and native read observed | Friend test results, date/length boundaries, keyboard errors, full mobile matrix | 1 / 5 |
| Private fields | Backend access tests pass | Each role on deployed service, attachments and exports when introduced | 1–5 |
| Atomic save and retry | Concurrent duplicate, changed payload and history rollback tests pass<br>A separate SQLite writer lock and a SQLite page-capacity limit each caused a local photo save to return 503 with no partial data, A local HTTP upload cut off mid-photo wrote nothing, After the successful retry and a forced process restart, the same key replayed the saved record and its photo remained readable | Verify multi-client retry and restart on Azure<br>The page-limit test exercises SQLite's full condition without filling the VM filesystem | 1 / 5 |
| Photo uploads | Found-item photo API tests cover the exact byte boundary, invalid/duplicate files, private reads, retry, transaction rollback, overlapping uploads and safe retry at the upload limit<br>Chrome against the Windows executable in local server mode verified the preview, same-key retry after a dropped response, one saved record, creator and admin access, and a campus user's denied photo request<br>Local standalone API accepted a 48 MP, 7.41 MB baseline 4:2:0 JPEG and stored 0.39 MB | Repeat in the native Windows client against Azure, verify lost-report photos there, and resolve DR-005 coverage for photos within the byte limit but above the profile-specific pixel limits | 0 / 2 |
| Lost reports and matching | Local tests cover mandatory report fields, owner and admin access, staff-assisted reports, private photos, rollback, exact retry and deterministic suggestions<br>The local directory walkthrough saved a campus report and found the expected item | Verify report and match flows against Azure, including multi-client retries, pagination and photo upload from native Windows | 2 |
| Claims | Local checks cover submission, claimant status, private answers, exact retry, distinct-key races, and history rollback<br>The item stays Open after submission | Azure client verification | 2 |
| Verification and decisions | Go tests cover evidence access, incomplete checks, competing approvals, rejection reason, and rollback on history and read-back failure<br>Chrome against the Windows executable in local server mode verified administrator evidence, the incomplete-check error, exact retry after a lost approval response, Open item status and claimant decision status | Deployed HTTPS and native window checks, evidence read failure and decision commit failure | 3 |
| On-site review and reopening | Go tests cover claimant-only requests, visibility to administrators, duplicate and repeated requests, rollback, reopening reason and permissions, guards for final items and competing claims, preserved history and fresh verification after reopening<br>Chrome against the Windows executable in local server mode verified the claimant request, administrator queue, reopening and renewed approval with both checks, Lost responses for both review actions replayed successfully | Native window and deployed client checks, plus ST-03 review of the procedure | 3 |
| Handover | Go tests cover holding-point authorization, minimal release details, required verification and signed receipt, Dubai time, concurrent releases, exact retry and rollback on disposition or history failures<br>Chrome against the Windows executable in local server mode verified the holding-point queue, form feedback, exact retry after a lost response, and the claimant's collection status | Native window and deployed client checks and ST-03 review of the paper receipt procedure | 3 |
| Custody and dispositions | Local Go tests cover transfer permissions, stale point, exact retry, holding-point handover eligibility, contact privacy, all final transitions, police reference validation, approval-versus-disposition race, and rollback<br>The holding-period test reopens an on-disk database and verifies 60-day and 30-day calculations and administrator-only visibility<br>Chrome against the Windows executable in local server mode verified transfer retry after a lost response, the deadline, contact entry, police reference validation, and opening the final item by reference | Native window and deployed client checks, team confirmation of the holding period and disposition procedure, Q-12/Q-13 remain open for approved claims | 4 |
| Backup and restore | For AC-NFR018, the current server backed up 500 found items, 300 lost reports, 20 claims, their history and 100 photos into a fresh local instance<br>Export and copy took 2.36 seconds on the test machine<br>Checksums matched, every column in all 21 tables matched, integrity and foreign-key checks passed, and the restored API opened all 100 photos for their owners and denied an unrelated account<br>The backup command also has tests for missing, empty or unrelated source files<br>Systemd is configured for the first and third Sundays at 02:00 Dubai time | Validate the first scheduled snapshot on Azure and perform the recovery drill from an off-VM copy | 0 / 5 |
| Directory withdrawal | Backend tests cover FR-017 removal from search, admin access, retained photo/history, retry and rollback<br>The administrator action is in the item view | Verify the action in the native window and against Azure, Approved-claim handling remains open under M3 Q-13 | 0 / 1 |
| Activity history | Local Go tests cover administrator-only access, returned and withdrawn records, related claim and handover entries, equal timestamps and fractional-second ordering, Lost-report history tests cover recorded details, retry, denied access and storage failure<br>The Windows executable in local server mode showed one creation event to the administrator in Chrome and denied the report owner | Native window and Azure verification for the history screens | 1 |
| Intake corrections | Go tests cover original receiver and administrator access, current holding point, required fields, stale edits, final records, rollback and exact retry<br>Chrome against the Windows executable in local server mode verified the prefilled edit form, retry after a lost response and old/new values in the administrator history | Native window and Azure checks | 1 |
| Our hosted service | Azure staging pulled commit `bfcce91`, built and checksummed the server, saved its pre-update database snapshot and passed the updater's health check<br>The public setup endpoint responded over HTTPS with signup enabled after the update<br>The Windows Azure build forwarded setup and session requests through native WebView2 against a local cookie fixture<br>Local deployment fixtures cover good and failed binary updates, rollback, concurrent update refusal and bad checksums | Verify authenticated remote workflows, independent clients sharing records, runtime memory, monitoring, and recovery from an off-VM backup | 0 / 5 |
| Windows release | Native WebView2 verified the Azure 1.0 build opens at username sign-in with Create account<br>The local preview passed account setup, campus sign-in and navigation through Windows accessibility controls<br>The first-sign-in tour moved and focused the real desktop app controls in the browser walkthrough, It opened the filters and report form, reached Claims and the staff handover queue, and cleared highlights on exit | Two-client Azure workflow, clean machine, installer/update behavior, signature/distribution decision, WebView prerequisite and upgrade checks | 5 |
| Web preview | Angular builds into the directory in `vercel.json`<br>Go tests verified precise web-origin checks, a Secure, HTTP-only cookie, and an authenticated save and session read through a local HTTPS rewrite | Deploy on a stable Vercel host, configure the exact origin in Azure, and test shared Windows/browser records, sign-in and photo uploads | 2 |
| Browser/mobile/accessibility | The local directory smoke test checked the 360px item view, keyboard expansion of filters, and return to the same focused result and scroll position, A 360px Chrome walkthrough used only keystrokes from search through claim confirmation, Computed text/background contrast for eight rendered sign-in, directory and claim samples ranged from 5.59:1 to 6.27:1 following the text colour update | Observed keyboard-only usability participant for AC-NFR017, deployed browser matrix and full WCAG 2.1 AA review, including screen-reader and contrast checks for other screens and states | 5 |
| Performance | The local contrast preview reached its first-launch setup screen in 4.27 seconds on one Windows machine, Five signed-in idle samples covered seven processes including WebView2, with 104.54–141.52 MiB private working set and 266.99–287.32 MiB private committed memory | Repeat cold and warm startup on the release build, measure RAM and CPU during activity, and complete the deployed M2 load checks | 5 |
| Usability | Guide prepared | Friend feedback, M2 participant protocol, 8/10 search-to-claim within 3 minutes, median intake at most 60 seconds | 5 |
| Dependencies and release security | Runtime npm audit clean in prior check | Resolve/review development advisories<br>Go dependency scan, threat review, secret/log checks and release recheck | 5 |

## M2 measurements still open

M2 section 3.3.1 defines the reference dataset as 500 found items, 300 lost reports, 200 claims and 100 stored 1 MB photos

The reference load is 20 concurrent signed-in users with a 70% search, 20% submission and 10% view mix, NFR-004 separately asks for the response checks at 2,000 found items, 2,000 lost reports, 1,000 claims and 200 stored 1 MB photos

Formal acceptance measures time through results or confirmation display on a UAE client and requires three passing runs

I ran one Windows loopback capacity check at the NFR-004 counts, including all 5,000 initial history entries, with 20 sessions completing 800 requests and zero HTTP errors

That included 20 found-item uploads of 1 MB each, with API p95 times of 28.2 ms for 560 searches, 160.2 ms for 54 found-item submissions, 33.4 ms for 53 lost-report submissions and 41.8 ms for 53 claims

I checked the saved record and photo totals afterward, with these timings covering HTTP completion on loopback

I also checked a 48 MP photo using the standalone API and `GOMEMLIMIT=384MiB`, which took 0.687 s to save and 0.017 s to replay, with server working set rising from 28.7 MiB after login to a peak of 135.6 MiB during upload

That sample covers the server on loopback, with Azure latency, concurrent users and the desktop process tree still needing their own measurements

An earlier run started with 500 found items and 100 photos of 1 MB each, then completed 200 requests from 20 staff sessions without errors

It covered 140 searches at a maximum of 28 ms, 20 views at 24 ms, 24 photo-free saves at 40 ms and 16 photo uploads at 358 ms, with every save present in the record, photo and history totals

The full NFR-001 to NFR-004 acceptance still needs the remaining record types, deployed client and three reference-load runs

In my earlier backup rehearsal, the 200.84 MB snapshot took 2.36 s while the live API completed 100 found-item saves, with the slowest save at 93 ms

After copying it into another data directory, the restored API was ready in 0.53 s, measured from after the copy and covering the found-item and photo part of M2

## Before I call it ready

I still need the implemented workflows to pass their permission, atomic-save, failure and deployment checks

That includes Windows and browser runs, restored backups, shared records and the M2 acceptance results

For open policy questions, I need a confirmed answer before enabling the affected operation

RIT isn’t providing hosting, so I’m using the Azure for Students VM for the API and still need the cost, persistence and recovery checks before release
