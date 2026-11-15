# Milestone and diagram coverage

M1 defines the domain, actors and scope. M2 v1.0 defines requirements and acceptance checks. M3 v1.0 defines how the system should behave. Use the PDFs as the source of truth and log discrepancies rather than silently changing the baseline.

## M1 scope and M2 trace

| M1 scope | Planned screens and service work | M2 functional requirements |
| --- | --- | --- |
| SC-01 Found item recording | Intake, optional photo and authorized correction of Open items | FR-001–002 |
| SC-02 Lost item reporting | My reports and authorized assisted visitor reporting/retrieval | FR-003–004 |
| SC-03 Directory | Signed-in search and public-field-only detail | FR-005–006, FR-017 |
| SC-04 Claim and verification | Submit, track, review evidence, decide, request review, reopen and record approved handover | FR-007–013, FR-028 |
| SC-05 Item status and custody | Transfer, contact attempts, holding period and permitted final disposition | FR-014–016, FR-018 |
| SC-06 Advisory matching | Compare permitted report/item attributes and show suggestions | FR-019–020 |
| SC-07 Login and roles | Labelled simulated credentials, server sessions and per-record permissions | FR-021–024 |
| SC-08 Activity history | Transactional append, admin chronological retrieval and repeat handling | FR-025–027 |

Use M2 Appendix A for the full crosswalk, including overlapping IDs. The [M2 register](m2-requirements.md) includes every requirement record, including interface, data, constraints and quality. Track future work and exclusions from M1 A4 and M2 §2.6 as scope decisions. Physical storage, identity checks and signed receipt handling stay manual. Police involvement uses manually recorded references. The project uses labelled test accounts and the M2 staff-assisted visitor workflow.

## Diagram checks

PDF pages below are one-based. The original figures remain in the source milestone PDFs. Link the implemented Go modules, Angular screens, storage constraints and test evidence to the figure IDs as work lands.

| Figure | What must be reflected in the build | Check to record |
| --- | --- | --- |
| M1 Fig. 1, p.11 and M2 Fig. 1, p.15 | Role and external-system boundaries, one shared record across both desks, simulated identity | Walk each actor's permitted input/output. Verify no live SSO or police integration. Reconcile management arrows with M3 DFD-0 and deferred reporting scope. |
| M1 Fig. 2, p.12 | Item held at exactly one point. Reports and advisory matches, multiple claims, at most one approved claim and at most one disposition | Verify relationships on create, transfer, approval and release. Check storage-level uniqueness and link integrity. |
| M3 DFD-0 part 1, p.9 | 1.0 intake/correction and 3.0 directory/withdrawal use D1. 2.0 report/retrieval uses D2. 6.0 computes suggestions using D1/D2. | Verify authorized inputs, record outputs, public projection, and no history event or write for computing suggestions. |
| M3 DFD-0 part 2, p.10 | 4.0 claims and handover use D3 and D1. 5.0 handles holding points, status and contact events. | Verify current holding point, allowed custody transitions, blocked invalid disposition and actor history. |
| M3 DFD-0 part 3, p.11 | 7.0 controls identities and roles in D4. 8.0 manages D5 append-only activity and authorized queries. | Direct forbidden-request tests, session revocation, history access and mutation rollback. |
| M3 DFD-1, p.14, balancing table pp.14–15 | 4.1 submit/request review, 4.2 verify, 4.3 decide/reopen, 4.4 release. DF-42 carries checks internally. Parent endpoints and child flows balance. | Cross-check DF-05, DF-19–27, DF-38 variants and DF-42 against §2.3. Test unauthorized, duplicate, rejected/reopened, approved and handover paths. |
| M3 CD-1.1, p.30 | Domain entities and multiplicities, including disposition, claim, handover, history and holding-period setting | Map each concept to Go types or schema relationships. Verify valid transitions, reference integrity and multiplicities. |
| M3 CD-1.2, p.31 | Services, permission policy, replaceable sign-in and store responsibilities | Map service/store boundaries to code. Check authorized access before protected reads/writes and transaction ownership. |
| M3 CD-1.3, p.31 | Item/claim states, roles, verification checks, outcomes, actions and other enums | Assert permitted values across API, storage and UI. Verify no invented Reserved state and no claim cancellation or expiry. |
| M3 CD-1.4, p.32 | Distinct authorized identity, public/private views, photo, verification, handover, disposition and change-event data | Verify data schemas, allowlists, size limits, required receipt fields and privacy, including rejection of identity-document fields. |
| M3 SD-1a, p.37 | Authorize evidence access, read claim and item, then return evidence or an explicit failure. Store only results of manual checks. | Forbidden, missing, read error and success paths. Check no protected read on forbidden request and no state change in evidence review. |
| M3 SD-1b, p.38 | Authorize the decision, check existing submission before routing to approval/rejection | Test denial, lookup failure, exact retry, changed payload with reused key, and both branches. |
| M3 SD-1c, p.39 | Re-read eligibility in the critical section. Require Open/Submitted and both checks. Save conditionally, append history, read back and commit. | Competing approvals yield one winner and no losing write or history. Inject history, read-back and commit failures. Item remains Open until handover. |
| M3 SD-1d, p.40 | Re-read claim, require Submitted and a nonblank reason, append history and confirm after commit | Verify guards, atomic failure, complete read-back, reason-only rejection and same-key retry after lost response. |

## Figure and policy follow-up

M3 Q-14 names missing claim-status and release-details operations/actions in CD-1.2/1.3, the missing key-mismatch refusal in SD-1b, and the implicit read-back return in SD-1c. Keep these explicit in implementation tests and reconcile the editable figure sources before release. The original editable PlantUML/draw.io sources were not found in the workspace. Use the PDF panels as reference and check with the team for the source files.

Q-10 is the hosting/storage decision for Stage 0. Q-11 is how the holding-period setting is loaded. Q-12 and Q-13 remain open for approved claims. Q-01 through Q-09 retain the M2 Appendix C.2 status pending confirmed answers. Report evidence against each requirement and record any proposed change with the affected model and test.

The deployment sketch below is a proposal for Stage 0 and Stage 2. D1–D5 stay logical store boundaries in M3. They can share a physical database and transaction boundary.

```mermaid
flowchart LR
    Browser[Desktop or mobile browser] -->|HTTPS| Web[Vercel Angular frontend candidate]
    Web -->|HTTPS API rewrite| Go[Shared Go service]
    App[Windows or Linux app] -->|HTTPS| Go
    Go --> Data[(Persistent records, history and private photos)]
    Data --> Backup[(Protected backups)]
```
