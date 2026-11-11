# Campus Lost & Found

**Find it. Verify it. Return it.**

Campus Lost & Found is being built for RIT Dubai as Team LAMNZ's final project for **SWEN 261 - Introduction to Software Engineering**, section 600, Fall 2026.

The project brings the reception/security desk and the Office of Student Affairs into one shared lost-property workflow: record an item, help its owner find it, verify the claim, and record the handover.

**Current status:** requirements review and design research. There is no runnable application yet. Windows and Linux builds are planned, with Angular used for the application interface and no Electron.

## 🧭 The workflow

* **Campus users:** search the found-item directory, report a loss, submit a claim and follow its decision.
* **Holding-point staff:** record found items, transfer custody and record approved handovers at their assigned holding points.
* **Administrators:** review ownership evidence, decide claims, manage dispositions and inspect the activity history.

These are planned capabilities from M2 and M3, not a claim that they are implemented.

## 🔒 The rules that matter

* Private photos and identifying details stay with the record creator and administrator. They are not directory thumbnails.
* An approved claim is not a returned item. Physical handover is a separate recorded action.
* Retrying a submission must not create another record. A slow connection is not a request for two claims.
* A save is confirmed only after storage succeeds. The record and its history must survive together.
* Sign-in uses labelled prototype accounts, as required by the course baseline. It does not connect to RIT SSO or ask for university passwords.

## 🛠 Development

Work is planned in **five iterations**, each reviewed by Mohammed before a GitHub push. Installation instructions and test results will be added when the corresponding implementation exists.

See [design research and iteration plan](docs/design-research.md) for the proposed UI, architecture, acceptance checks and open decisions.

## 📚 Course context

**Team:** LAMNZ  
**Course:** SWEN.261.600, RIT Dubai  
**Instructor:** Dr. Saleh Aldaajeh  
**Team members:** Mohammed Al Ariefy, Abdullah H. Ansari, Lana Al Tajer, Zahra Ibrahim and Natnael Argaw.

This is a course project. Official campus deployment and stakeholder approval are not claimed.
