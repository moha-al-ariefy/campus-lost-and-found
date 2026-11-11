# Campus Lost & Found - design research and build plan

Research date: 11 November 2026. Status: proposed direction; no application implemented or performance results claimed.

## 1. What the project actually needs

Sources: M1 (1).pdf (domain analysis, 31 pages), M2 (1).pdf (SRS, 69 pages), M3 (1).pdf (design description, 56 pages), supplied in the workspace. M2 is the requirements baseline. M3 supplies the service boundaries, data flows, transaction rules and open design questions.

This is a shared campus service, not a separate inventory on every laptop. Reception/security and Student Affairs need the same records. The app supports campus users, holding-point staff and administrators. Its eight capabilities are found-item intake, lost reports, directory search, claims and handover, item status and holding points, advisory matching, prototype sign-in and activity history.

Important implications:

- Search requires sign-in. Public details mean visible to signed-in users, not anonymous internet access.
- Directory results expose item ID, category, general location, date found, holding point and status. Photos, identifying details and contact details must not be sent to unauthorized clients. Use category icons instead of public item photos.
- Private fields need a conservative explicit allowlist. Free-text descriptions must not accidentally disclose ownership clues.
- Staff assignment does not grant access to another creator's identifying details. The administrator has the evidence-review role.
- Claim approval requires both identifying-detail and in-person ID checks. Do not store ID scans or ID numbers.
- Approval leaves the item Open. Handover, Returned disposition and history are saved together. Only assigned holding-point staff complete handover under the baseline permission list.
- Matching is deterministic: same category, ranked by equal non-blank color, brand and building, then item ID. Never describe it as AI or show an invented confidence percentage.
- Failed reads, empty results and unconfirmed writes are different states. Preserve entered data and allow checking/retrying the same submission.
- No automatic disposal, record deletion, email/SMS notifications, real SSO, police integration or AI image matching in the baseline.
- The browser interface remains required, including 360px-wide mobile layouts. Windows/Linux desktop support extends this baseline.

## 2. Research and how it changes the design

### Desktop navigation

[Microsoft navigation guidance](https://learn.microsoft.com/windows/apps/design/basics/navigation-basics) recommends list/details layouts for frequent movement between records and data-entry work. Apply this to staff queues and administrator claim review: selectable records on the left, full details on the right, retaining filters and scroll position. On narrow windows, use a full detail page with a clear Back action.

### Forms and decisions

[GOV.UK error summaries](https://design-system.service.gov.uk/components/error-summary/) support a linked summary alongside errors at individual fields. Use both, preserve non-secret input and move focus to the error summary on failed validation.

[GOV.UK check answers](https://design-system.service.gov.uk/patterns/check-answers/) supports reviewing information before a consequential submission. Use a concise confirmation for handover and final disposition. Avoid adding an extra page to routine intake unless usability tests justify it.

[GOV.UK multiple-task guidance](https://design-system.service.gov.uk/patterns/complete-multiple-tasks/) is intended for longer transactions spanning tasks or sessions. Intake should remain a short grouped form; claim progress can show clear statuses without making users navigate a large task checklist.

### Visual identity and accessibility

[RIT accessibility guidance](https://www.rit.edu/brandportal/principle-1-perceivable) distinguishes brand orange from accessible orange and requires appropriate labels and contrast. Use an RIT-inspired restrained orange accent, dark readable text and neutral surfaces, with contrast verified for each actual foreground/background pair. Do not assume orange automatically works for small text or white button labels.

[W3C target-size guidance](https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum) and [focus visibility guidance](https://www.w3.org/WAI/WCAG22/Understanding/focus-not-obscured-minimum) inform additional usability checks. Prefer 44px primary controls, clear focus rings and no sticky region obscuring keyboard focus. M2's formal baseline remains WCAG 2.1 AA; these extra checks do not constitute a certification claim.

### Angular and desktop runtime

[Tauri webview documentation](https://v2.tauri.app/reference/webview-versions/) confirms Windows uses WebView2 and Linux uses WebKitGTK. Tauri is a credible non-Electron candidate, but it is still a webview application and does not guarantee a RAM target. Validate both engines and include child-process memory in measurements.

[Angular performance guidance](https://angular.dev/best-practices/runtime-performance) describes profiling and controlling unnecessary change detection. Use lazy-loaded feature routes, bounded result pages, small component trees and measured updates. Do not load all records, all photos or a second UI framework at startup.

These sources provide useful patterns, not proof of a universally best UI. The final choice is validated against the project's actual user tasks.

## 3. Proposed interface

Visual direction: light neutral workspace, dark text, restrained orange primary action, subtle borders, modest corner radii and system fonts. Use whitespace to group information rather than giant cards. Motion should clarify changes, respect reduced-motion settings and never delay a task.

| Area | Main content | Primary action |
| --- | --- | --- |
| Campus directory | Category, location, date and holding-point filters; compact results with safe category icons | View item / start claim |
| My reports | Own lost reports and possible matches | Report a lost item |
| My claims | Submitted, Approved or Rejected decisions with clear next step | View claim / request review |
| Staff intake | Required item and receipt fields first; optional details grouped below | Save found item |
| Staff holding point | Items in custody and approved releases; persistent filters | Record handover / transfer |
| Admin claims | Queue and evidence detail side by side; two explicit verification checks | Record decision |
| Admin item detail | Status, custody, contact attempts and chronological history | Applicable record action |

Keep role-specific navigation short. Separate My claims from administrator review to avoid confusion. Display the current role and holding point where relevant.

Item state and claim state must have distinct labels. An Approved claim may show the next step 'Collect at reception', but the underlying item remains Open until staff records handover. Do not introduce a Reserved item status.

Forms use visible labels, useful examples, explicit optional markers and one clear main action. Private fields explain who can see them. Photo upload supports preview/removal and explicit failure feedback, only in permitted views. Stored photos follow the baseline limit: one JPEG/PNG, accepted input up to 10 MB, stored at 1 MB or less.

Required states on every data screen: loading, loaded, empty, filtered-empty, unavailable, permission denied and session expired. Mutations also need validation failure, pending, confirmed success and outcome unconfirmed. No success toast before a confirmed durable save.

## 4. Proposed architecture

Recommendation: Angular application shared between browser and Tauri 2 desktop shells, backed by one centrally authoritative API and transactional database. A TypeScript service layer is a candidate that fits the observed AUEV style and M3's class/interface design; select the server framework and database driver after checking current support and local tooling.

Desktop clients must use the shared server for campus records. Separate per-installation databases would break shared custody, concurrent approval and authoritative history. Provide a fully functional local server/database setup for course demonstration and restore testing; label it clearly as a separate local instance. Do not silently switch to offline writes or invent synchronization.

Evaluate PostgreSQL for authoritative storage: transaction boundaries, unique constraints and locking must enforce one approved claim, one disposition, atomic history, and request idempotency. A small native desktop shell avoids bundling a separate server into every client. Hosting remains an unresolved zero-budget deployment choice; check capacity, storage and sleep behaviour against M2 before selecting a service.

Prototype credentials still need real verification and revocable server-side sessions. Keep the IdentityProvider replaceable. Return only allowed fields from the server, including in search and match results. Secure uploaded images and export files using the same permissions.

## 5. Five iterations

Each iteration ends with a runnable increment, requirement coverage, actual test evidence, known limitations and the user's review. GitHub pushes happen only after explicit approval. This research/setup work precedes iteration 1.

| Iteration | Working increment | Review evidence |
| --- | --- | --- |
| 1. Foundation and first real workflow | Angular navigation, real test-account sessions and permissions, central persistence, staff intake, public-safe directory, atomic history and retry handling from the first write; initial desktop shell | Create an item, restart, find it from another session; reject unauthorized data access; initial Windows startup/RAM baseline |
| 2. Reports and ownership claims | Lost reports, private photo handling, deterministic match suggestions, claim submission/status, duplicate protection | Real report-to-claim journey; invalid uploads and interrupted requests; mobile and keyboard checks |
| 3. Verification and handover | Administrator evidence review, checks, approval/rejection, review request/reopen, assigned-staff release | Competing approvals, unauthorized decisions, idempotent handover and rollback on history/storage failure |
| 4. Custody and recovery | Transfers, withdrawal, permitted dispositions, contact attempts, holding-period configuration, full history, export/restore; resolve policy questions before affected behaviour | State-transition tests, restoration with all records/photos, review of Q-12/Q-13 decisions |
| 5. Release and measured usability | Windows/Linux packages, browser matrix, accessibility, performance/load checks, usability fixes and course documentation | Install/run evidence on both OSs; measured RAM/startup; RD/RL results; participant evidence where available; final reviewed release |

No iteration may advertise a later feature as working. Test fixtures are explicitly separated from live data.

## 6. Acceptance and measurement

Retain M2 acceptance checks and map implemented requirements to tests. Selected targets include 20 concurrent signed-in users; capacity for 2,000 found items, 2,000 lost reports, 1,000 claims and 200 stored 1 MB photos; 95% of report/claim submissions confirmed within 3 seconds under RD/RL; at least 8 of 10 first-time participants completing search-to-claim within 3 minutes; median staff intake at most 60 seconds; and a documented local setup within 15 minutes.

The performance harness must reproduce the exact RD/RL conditions from M2 section 3.3.1. Do not substitute these selected targets for the full SRS. Automated tests cannot stand in for participant testing.

Desktop measurements: release builds, stated OS/hardware/webview version, cold start, warm start, idle memory, directory browsing, image upload, repeated navigation and a sustained session. Include the whole process tree; distinguish private memory and working set where available. Record CPU and slow interactions. Establish a realistic numeric RAM budget from iteration 1 measurements; none is agreed yet.

## 7. Decisions still open

- Use the user's existing Git installation for version control. The mistakenly downloaded GitHub CLI and archive have been removed.
- Repository: https://github.com/moha-al-ariefy/campus-lost-and-found (private). The user authorized publishing the current project documents on 11 November 2026. Subsequent implementation pushes still require review.
- Q-10: zero-budget hosting/storage needs evaluation against transactional and capacity requirements.
- Q-11: implement holding-period loading through documented configuration, not a new administration screen. No policy duration is invented.
- Q-12: M3 does not allow revoking an approved but uncollected claim; CH-09 is proposed, not approved. Preserve that limitation until a decision is recorded.
- Q-13: withdrawing an item with an approved claim can strand the claimant. This requires an explicit policy decision before implementing the affected path.
- Q-14: implement claimant decision retrieval and pre-release staff details required by the text even though the diagrams lack operations. Document the mapping and missing diagram updates.
- Q-01 through Q-09 remain unresolved in the supplied documents. Do not claim stakeholder approval, university deployment or a validated legal policy.

## 8. Style reference

Reviewed [AUEV README](https://github.com/moha-al-ariefy/AUEV-AI-CODING-TOOL/blob/main/README.md), [auditManager.ts](https://github.com/moha-al-ariefy/AUEV-AI-CODING-TOOL/blob/main/vscode/src/auditManager.ts) and [ghostSanitizer.ts](https://github.com/moha-al-ariefy/AUEV-AI-CODING-TOOL/blob/main/vscode/src/ghostSanitizer.ts).

Observed style: descriptive camelCase operations and PascalCase types, explicit types, early returns, short explanatory comments, and occasional dry humour tied to a real edge case. Documentation uses clear feature sections, restrained emoji, bold labels and runnable commands. Follow those traits while keeping status and evidence honest. Do not copy unrelated security-suite marketing or claims.
