# Campus Lost & Found

I built this with Team LAMNZ for our RIT Dubai **SWEN 261 final project**, section 600, Fall 2026

The idea is to give Reception and Student Affairs one shared record for each item, from the moment someone hands it in until its owner collects it or staff record another outcome

## What I used

Angular handles the interface, Wails gives us the Windows EXE, and the Go backend stores records and private photos in SQLite

The shared API runs on Azure, with Windows and the web as our release targets

## What works so far

- Found-item intake and corrections, including private photos and finder details
- Directory search by name, category, location, date and holding point
- Lost reports and possible matches based on the item's attributes
- Claims, duplicate protection, admin verification and decisions
- On-site review requests and reopening rejected claims
- Approved handovers with verification and a signed paper receipt reference
- Custody transfers, contact attempts, final outcomes and activity history
- Username signup, course accounts, server sessions and role checks
- A walkthrough that takes you through the actual controls

I checked these locally with the Go suite and Windows/browser walkthroughs, and I keep the remaining Azure, participant and release checks in the [test map](docs/test-map.md)

## How the roles work

Campus users can search, report a loss and follow their claims

Staff record items and hand them over at their assigned holding point, while administrators review evidence, decide claims and manage final outcomes

Photos, finder details and ownership clues stay with the creator and administrator, so the public listing gives people enough to recognise an item while keeping the verification details private

Approval and collection are separate steps, and I only confirm a save after the record and its history have committed together

The course requires simulated accounts under CON-002, so use the app's own credentials and keep your university password separate

## Running the app

For the shared build, extract the Azure Windows ZIP and open `CampusLostFound.exe`, then sign in or choose **Create account**

The source build outputs `desktop/build/bin/CampusLostFound.exe` and needs WebView2 Runtime

If you build locally without a server address, the app asks you to create the local course accounts, then you can sign in as `student`, `reception`, `affairs` or `admin` with the password you chose

Local records go in `%APPDATA%/CampusLostFound/campus.db`, or the directory set by `CAMPUS_DATA_DIR`

For a shared build, pass `-ldflags "-X main.packagedServerURL=https://your-api-host"` to Wails, or set `CAMPUS_SERVER_URL` when running it

## Building from source

You'll need Node 22.12+ within version 22 or Node 24+, Go 1.26+, Wails CLI 2.17.0 and Windows WebView2

```sh
npm ci
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
npm run desktop:build
```

Add the Go bin directory to PATH, or use the workspace CLI in `tmp/tools/bin`

For browser development, run `npm run build:desktop-ui`, set `CAMPUS_ALLOW_SETUP=true` for a new local database and run `npm run dev`, then open `http://127.0.0.1:4200`

Turn setup off once the accounts exist

I use `npm test` for backend tests and `npm run check` for Go vet

## Server

Build the API from `desktop` with `go build ./cmd/campus-server`, using an absolute `CAMPUS_DATA_DIR` on persistent storage

The API listens on `127.0.0.1:3000` behind the HTTPS proxy, while the desktop executable also supports `--serve` for a local service

The templates are in `desktop/deploy`, with `APP_ORIGIN` for the public API origin and `CLAF_API_HOST` for Caddy

The server roster is initialised with `-init-test-accounts` and four distinct passwords on standard input, ordered student, reception, affairs and admin

Set `CAMPUS_SIGNUP=true` to enable campus-user registration, which I added alongside the course account workflow

Google sign-in is still pending

Uploads accept one JPEG or PNG up to 10 MB per found item or lost report, then store at most 1 MB per photo with the record

The decoder supports up to 50 megapixels for baseline 4:2:0 JPEG and 32 megapixels for the other supported images

The backup command takes an absolute new file path through `-backup`, including private photos in the SQLite snapshot

The systemd templates schedule backups on the first and third Sundays at 02:00 Dubai time in `/var/lib/claf-backups`, with the off-VM recovery check still tracked in the test map

Admins can withdraw an incorrect Open listing with a reason, keeping its record, photo and history

I kept withdrawal blocked for approved claims while M3 Q-13 is unresolved

## Group testing

Run `./scripts/package-test.ps1` in PowerShell after building

For the named Azure build, use `./scripts/package-test.ps1 -BuildName campus-lost-found-azure-windows-x64 -ExecutableName CampusLostFound-AzureCandidate.exe`

The ZIP in `release/` includes the EXE, logo, guide, feedback form, test map and checksum, with another checksum next to the ZIP

Send both files together and follow the [short guide](docs/TEST-GUIDE.md)

I keep the [M1–M3 diagram coverage](docs/milestone-coverage.md), [M2 register](docs/m2-requirements.md) and [original iteration 1 results](docs/iteration-1.md) here so we can check what is actually done before calling it finished

## Course

Team LAMNZ, SWEN.261.600, RIT Dubai, Fall 2026

Our instructor is Dr. Saleh Aldaajeh

Team members are Mohammed Al Ariefy, Abdullah H. Ansari, Lana Al Tajer, Zahra Ibrahim and Natnael Argaw

I built this for and as our course final project, with official campus deployment and stakeholder approval still outside what we have confirmed
