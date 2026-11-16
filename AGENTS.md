# Campus Lost & Found - working rules

This is Team LAMNZ's final project for RIT Dubai, SWEN 261: Introduction to Software Engineering, section 600, Fall 2026.

- Read M1, M2 and M3 before changing domain behaviour. M2 is the requirements baseline; M3 is the design. Track changes and unresolved questions explicitly.
- Deliver the demo feedback and the full agreed M1–M3 scope. Trace each domain change to the relevant UML classes, sequence branches and data flows. Verify successful, denied, retried and failed requests where each diagram calls for them.
- Keep permissions, privacy and durable writes at the server boundary. Choose code structures that fit the task and the existing code. Avoid mechanically repeating one pattern across unrelated features. Keep responsibilities clear, bound expensive work, and test the course capacity and concurrency targets before making scalability claims.
- The app must work. No fake saves, invented statistics, pretend integrations, decorative buttons or hardcoded business data presented as live data. Test fixtures belong in explicitly labelled test/demo environments.
- Build Windows and Linux versions. Do not use Electron. Angular must implement meaningful application functionality.
- The Windows deliverable must be an actual .exe. The current implementation uses Wails with Angular and a native Go/SQLite backend. Browser testing is supplementary, not the Windows deliverable. Benchmark the whole process tree before claiming the architecture is lightweight.
- Treat RAM, startup time and responsiveness as measured requirements. Include webview child processes in desktop measurements. Never claim performance or platform support without evidence.
- Preserve the baseline browser/mobile workflows as well as the requested desktop builds.
- Work in this order: Stage 0 shared server and private image storage, Stage 1 finish the EXE, Stage 2 finish the web version, Stage 3 ship. Stop at each stage for the user's review. The EXE stage can have smaller review increments. Never push to GitHub without explicit user approval for that push.
- Use the user's existing Git installation for version control. Do not download or install GitHub CLI. Repository creation does not authorize pushing project content.
- Follow the writing and code style observed in moha-al-ariefy/AUEV-AI-CODING-TOOL: descriptive names, explicit types, early guards, short purpose-driven comments, occasional dry humour where useful. The user explicitly requires no emojis in the README. Keep documentation direct and practical. Never invent release badges, results or installation commands.
- Finish and verify coherent batches. Show the proposed commit subject in the AUEV repository's style and wait for explicit approval before creating that commit. Approval to commit does not approve a push.
- Explain in README that this was built for and as the RIT Dubai SWEN course final project. Do not imply official university deployment or endorsement.
- Prototype accounts are an explicit course requirement (CON-002). Implement actual credential verification, server-side sessions and permissions; label the login simulation and never request university passwords.
- Enforce permissions and public/private field separation on the server. UI hiding is not authorization.
- Keep business changes, append-only history and retry handling within one atomic storage boundary. Verify concurrent approvals, repeated requests, lost responses and storage failures.
- Never silently introduce cancellation, expiry or withdrawal policy for approved claims. Track M3 Q-12 and Q-13 as unresolved policy decisions.
- Keep all user-facing text in a language resource file. Preserve keyboard operation, responsive layouts and the baseline accessibility requirements.
- No sub-agents unless explicitly requested by the user or another applicable instruction.
- Production readiness is the required release goal. Track unmet release gates honestly in docs/test-map.md. RIT does not provide hosting; plan our own externally hosted shared service, not separate local databases as the production architecture.

Keep hosting setup guides, research and working notes outside the repository. Use docs/milestone-coverage.md for the M1/M2/M3 diagrams and docs/m2-requirements.md for the full M2 baseline index. Track evidence before marking checks complete.
