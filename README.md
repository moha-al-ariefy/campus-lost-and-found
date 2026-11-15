# Campus Lost & Found

**Find it. Verify it. Return it.**

Campus Lost & Found is being built for RIT Dubai as Team LAMNZ's final project for **SWEN 261 - Introduction to Software Engineering**, section 600, Fall 2026.

The project brings the reception/security desk and the Office of Student Affairs into one shared lost-property workflow: record an item, help its owner find it, verify the claim, and record the handover.

**Current status:** iteration 1 is ready for review. The Windows executable runs Angular in a system webview with a Go backend and SQLite storage. No Electron. Linux remains a required target; it has not been built or verified yet.

Working now: course-account sign-in, server-side sessions and permissions, found-item intake, filtered directory, private intake details, administrator history and atomic retry-safe saves. Reports, photos, claims and handovers belong to later iterations.

## The workflow

* **Campus users:** search the found-item directory, report a loss, submit a claim and follow its decision.
* **Holding-point staff:** record found items, transfer custody and record approved handovers at their assigned holding points.
* **Administrators:** review ownership evidence, decide claims, manage dispositions and inspect the activity history.

These are planned capabilities from M2 and M3, not a claim that they are implemented.

## The rules that matter

* Private photos and identifying details stay with the record creator and administrator. They are not directory thumbnails.
* An approved claim is not a returned item. Physical handover is a separate recorded action.
* Retrying a submission must not create another record. A slow connection is not a request for two claims.
* A save is confirmed only after storage succeeds. The record and its history must survive together.
* Sign-in uses labelled prototype accounts, as required by the course baseline. It does not connect to RIT SSO or ask for university passwords.

## Development

Work proceeds through four stages: shared server, finished EXE, web version and production release. Mohammed reviews each stage and approves GitHub pushes explicitly. The Windows preview and standalone API source are available here. Azure deployment is pending.

### Run the Windows build

Open `desktop/build/bin/CampusLostFound.exe`. WebView2 Runtime is required. On a new local instance, choose a course test password, then sign in as `student`, `reception`, `affairs` or `admin`. Never use your university password. These accounts deliberately share a test password for this labelled course simulation.

Local records are stored in `%APPDATA%/CampusLostFound/campus.db`. `CAMPUS_DATA_DIR` overrides the directory. Separate local instances do not synchronize. The development test database is ignored by Git and is not embedded in the executable.

### Build from source

Requirements: Node 22.12+ within version 22, or Node 24+; Go 1.26+; Wails CLI 2.17.0; Windows WebView2. Linux also requires the platform dependencies in the [Wails installation guide](https://wails.io/docs/gettingstarted/installation/).

```sh
npm ci
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
npm run desktop:build
```

Add the Go bin directory to PATH. The build wrapper also recognizes the workspace-local CLI in `tmp/tools/bin`. Angular and Go are bundled into the executable; Node is a build dependency.

For browser development, run `npm run build:desktop-ui` first, set `CAMPUS_ALLOW_SETUP=true` for a new local test database, then run `npm run dev`. Open `http://127.0.0.1:4200`. Disable setup after initialization. `npm test` runs backend tests; `npm run check` runs Go vet.

The executable accepts `--serve` to run the local HTTP service at `127.0.0.1:3000`. Public hosting, HTTPS deployment and the desktop remote-service mode still need integration validation. Do not expose the local setup server publicly.

For staging, build the standalone Go API with `go build ./cmd/campus-server` from the `desktop` directory. It runs without Wails and requires an absolute `CAMPUS_DATA_DIR` on persistent storage. It listens on `127.0.0.1:3000` for an HTTPS reverse proxy and issues Secure session cookies. The staging command has no web account setup. Initialize the course roster locally on the server using `-init-test-accounts` and four distinct test passwords on standard input, ordered student, reception, affairs, admin.

### Verification

Nine Go backend tests pass. The release Windows executable has been opened, signed in and used to read a persisted item. Initial whole-process-tree measurement: 87.09 MiB private working set, 260.20 MiB private committed memory, 410.40 MiB summed working sets across seven processes. Summed working sets include shared pages. This is one sample, not a performance guarantee or a comparison with Electron.

See [iteration 1 evidence and limitations](docs/iteration-1.md).

### Friend testing

After building, run `powershell -File scripts/package-test.ps1`. The ZIP in `release/` contains the Windows executable, logo, [short test guide](docs/TEST-GUIDE.md), feedback form, [test map](docs/test-map.md) and executable checksum. It contains no database or preset password. Send the ZIP and its adjacent SHA256 file together. This is an unsigned Windows x64 local preview; Linux and shared-server testing are pending.

RIT hosting is unavailable. We will run our own shared service on an external host. Provider selection and deployment remain open. Production readiness is required and tracked in the test map; this preview is not the production release.

See [milestone and diagram coverage](docs/milestone-coverage.md) and the [M2 requirements register](docs/m2-requirements.md) for the course baseline and planned acceptance checks.

## Course context

**Team:** LAMNZ  
**Course:** SWEN.261.600, RIT Dubai  
**Instructor:** Dr. Saleh Aldaajeh  
**Team members:** Mohammed Al Ariefy, Abdullah H. Ansari, Lana Al Tajer, Zahra Ibrahim and Natnael Argaw.

This is a course project. Official campus deployment and stakeholder approval are not claimed.
