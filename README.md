# Campus Lost & Found

**Find it. Verify it. Return it.**

Campus Lost & Found is being built for RIT Dubai as Team LAMNZ's final project for **SWEN 261 - Introduction to Software Engineering**, section 600, Fall 2026.

The project brings the reception/security desk and the Office of Student Affairs into one shared lost-property workflow: record an item, help its owner find it, verify the claim, and record the handover.

**Current status:** Windows previews use Angular in the system webview, with a Go backend and SQLite storage. Release targets are Windows and the web. Azure hosts the shared backend on Linux.

Working now: course-account sign-in, server-side sessions and permissions, found-item intake with an optional private photo, filtered directory, private intake details, administrator history and atomic retry-safe saves. Lost reports, claims and handovers remain in development.

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

Work proceeds through four stages: shared server, finished EXE, web version and production release. Mohammed reviews each stage and approves GitHub pushes explicitly. The Windows preview and standalone API source are available here. The staging API is reachable over HTTPS. See the [test map](docs/test-map.md) for checks still due.

### Run the Windows build

Open `desktop/build/bin/CampusLostFound.exe`. WebView2 Runtime is required. On a new local instance, choose a course test password, then sign in as `student`, `reception`, `affairs` or `admin`. Never use your university password. These accounts deliberately share a test password for this labelled course simulation.

Local records are stored in `%APPDATA%/CampusLostFound/campus.db`. `CAMPUS_DATA_DIR` overrides the directory. Separate local instances do not synchronize. The development test database is ignored by Git and is not embedded in the executable.

Shared-server builds can include a default HTTPS API address by passing `-ldflags "-X main.packagedServerURL=https://your-api-host"` to Wails. `CAMPUS_SERVER_URL` overrides that address. The sign-in screen shows **Create account** when the connected service enables campus signup.

### Build from source

Requirements for the Windows build include Node 22.12+ within version 22 or Node 24+, Go 1.26+, Wails CLI 2.17.0 and Windows WebView2.

```sh
npm ci
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
npm run desktop:build
```

Add the Go bin directory to PATH. The build wrapper also recognizes the workspace-local CLI in `tmp/tools/bin`. Angular and Go are bundled into the executable; Node is a build dependency.

For browser development, run `npm run build:desktop-ui` first, set `CAMPUS_ALLOW_SETUP=true` for a new local test database, then run `npm run dev`. Open `http://127.0.0.1:4200`. Disable setup after initialization. `npm test` runs backend tests; `npm run check` runs Go vet.

The executable accepts `--serve` to run the local HTTP service at `127.0.0.1:3000`. Keep the local setup server on loopback. See the test map for the remaining staging checks.

For staging, build the standalone Go API with `go build ./cmd/campus-server` from the `desktop` directory. It runs without Wails and requires an absolute `CAMPUS_DATA_DIR` on persistent storage. It listens on `127.0.0.1:3000` for an HTTPS reverse proxy and issues Secure session cookies. The staging command has no web account setup. Initialize the course roster locally on the server using `-init-test-accounts` and four distinct test passwords on standard input, ordered student, reception, affairs, admin.

The production configuration templates are in `desktop/deploy`. Set `APP_ORIGIN` to the public HTTPS origin in the service environment and `CLAF_API_HOST` to its hostname when loading the Caddy configuration. Found items and lost reports accept one JPEG or PNG of at most 10 MB per record. The API stores at most 1 MB per photo in SQLite with the record and history. The decoder limit is 50 megapixels for baseline 4:2:0 JPEG and 32 megapixels for other supported images. Use an absolute new file path with `-backup` for a consistent SQLite snapshot that includes photos.

The `campus-backup.timer` and `campus-backup.service` templates schedule a verified private SQLite snapshot on the first and third Sundays at 02:00 Dubai time. Snapshots are stored in `/var/lib/claf-backups`. Keep an off-VM copy and verify that it can be restored.

Administrators can withdraw incorrect listings from the item view with a required reason. The database retains the record, photo and history. An approved claim blocks withdrawal while M3 Q-13 remains open.

Set `CAMPUS_SIGNUP=true` in the hosted service to enable username-and-password registration for campus roles. Signup is an extension to the original course workflow. The default simulated account mode remains available for CON-002. Google sign-in is pending.

### Verification

Run `npm test` for the Go backend suite. The test map records which workflows passed in the local preview and which still need verification against Azure.

See [iteration 1 evidence and limitations](docs/iteration-1.md).

### Friend testing

After building, run `./scripts/package-test.ps1` in PowerShell. The ZIP in `release/` contains the Windows executable, logo, [short test guide](docs/TEST-GUIDE.md), feedback form, [test map](docs/test-map.md) and executable checksum. Send the ZIP and its adjacent SHA256 file together. This is an unsigned Windows x64 preview. Check the test map for shared-server verification.

For the named Azure build, run `./scripts/package-test.ps1 -BuildName campus-lost-found-azure-windows-x64 -ExecutableName CampusLostFound-AzureCandidate.exe`.

RIT hosting is unavailable. The team runs its shared staging service on Azure. The test map tracks the remaining release checks.

See [milestone and diagram coverage](docs/milestone-coverage.md) and the [M2 requirements register](docs/m2-requirements.md) for the course baseline and planned acceptance checks.

## Course context

**Team:** LAMNZ  
**Course:** SWEN.261.600, RIT Dubai  
**Instructor:** Dr. Saleh Aldaajeh  
**Team members:** Mohammed Al Ariefy, Abdullah H. Ansari, Lana Al Tajer, Zahra Ibrahim and Natnael Argaw.

This is a course project. Official campus deployment and stakeholder approval are not claimed.
