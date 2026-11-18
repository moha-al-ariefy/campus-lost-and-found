# Campus Lost & Found - working rules

I’m building this with Team LAMNZ for our RIT Dubai SWEN 261 final project, section 600, Fall 2026

These are the rules I want followed while working on it

- Read M1, M2 and M3 before changing domain behaviour, using M2 as the requirements baseline and M3 as the design, and record changes and unresolved questions
- Deliver the demo feedback and full agreed M1–M3 scope, tracing domain changes to the UML classes, sequence branches and data flows, with successful, denied, retried and failed requests checked wherever the diagrams call for them
- Keep permissions, privacy and durable writes on the server, choose structures that fit the existing code, keep responsibilities clear and bound expensive work
- Test the course capacity and concurrency targets before making scalability claims
- The app must work, with real saves and integrations, working buttons and actual live data, while fixtures belong in clearly labelled test/demo environments
- Build the Windows app and web version with meaningful Angular functionality, keeping the Azure backend on Linux and Linux desktop packaging out of scope
- Use Wails with Angular and the native Go/SQLite backend, produce an actual Windows .exe and keep Electron out of this project
- Browser checks supplement the Windows deliverable, with RAM, startup and responsiveness measured across the whole process tree including WebView children
- Keep performance and platform claims tied to evidence
- Preserve the browser/mobile workflows alongside the desktop build
- Follow my order of Stage 0 shared server and private images, Stage 1 finish the EXE, Stage 2 finish the web version, then Stage 3 ship
- Stop for my review at each stage, with smaller review batches allowed during the EXE stage
- Use my existing Git installation and keep GitHub CLI out of the setup, with repository creation and content pushes treated as separate actions
- Use my style from moha-al-ariefy/AUEV-AI-CODING-TOOL, with descriptive names, explicit types, early guards, short comments and the occasional dry joke where it fits
- Keep the README free of emojis, make the docs direct and practical, and only include real badges, results and installation commands
- Finish and verify a coherent batch, show me its proposed commit subject in my AUEV style and wait for my approval before committing
- Get my explicit approval for each push, even when I’ve already approved the commit
- Add focused unit tests for new behaviour, run the affected suite between batches and the full suite before proposing a commit
- Say in the README that I built this for and as our RIT Dubai SWEN final project, without implying official university deployment or endorsement
- Keep the CON-002 prototype accounts with real credential checks, server sessions and permissions, label the login simulation and never ask for university passwords
- Enforce roles and public/private field separation on the server, since hiding a control in the UI cannot enforce access
- Keep each business change, append-only history and retry handling inside one atomic storage boundary, checking concurrent approvals, repeated requests, lost responses and storage failures
- Keep M3 Q-12 and Q-13 open until decided, without silently adding cancellation, expiry or withdrawal policy for approved claims
- Put user-facing text in the language resource file and preserve keyboard access, responsive layouts and the baseline accessibility requirements
- Use sub-agents only when I or another applicable instruction explicitly asks for them
- I want this ready for release, with unmet gates recorded honestly in docs/test-map.md and our own hosted shared service because RIT isn’t providing hosting

Keep hosting guides, research and working notes outside the repo

Use docs/milestone-coverage.md for M1/M2/M3 diagram coverage and docs/m2-requirements.md for the full M2 baseline, recording evidence before marking a check complete

Write the docs from my point of view, with natural wording and fewer short, full-stop-heavy sentences
