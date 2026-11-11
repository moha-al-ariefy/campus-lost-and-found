# Campus Lost & Found - working rules

This is Team LAMNZ's final project for RIT Dubai, SWEN 261: Introduction to Software Engineering, section 600, Fall 2026.

- Read M1, M2 and M3 before changing domain behaviour. M2 is the requirements baseline; M3 is the design. Track changes and unresolved questions explicitly.
- The app must work. No fake saves, invented statistics, pretend integrations, decorative buttons or hardcoded business data presented as live data. Test fixtures belong in explicitly labelled test/demo environments.
- Build Windows and Linux versions. Do not use Electron. Angular must implement meaningful application functionality.
- Treat RAM, startup time and responsiveness as measured requirements. Include webview child processes in desktop measurements. Never claim performance or platform support without evidence.
- Preserve the baseline browser/mobile workflows as well as the requested desktop builds.
- Work in five reviewable iterations. Stop at each iteration for the user's review. Never push to GitHub without explicit user approval for that push.
- Use the user's existing Git installation for version control. Do not download or install GitHub CLI. Repository creation does not authorize pushing project content.
- Follow the writing and code style observed in moha-al-ariefy/AUEV-AI-CODING-TOOL: descriptive names, explicit types, early guards, short purpose-driven comments, occasional dry humour where useful. README sections can use restrained emoji, bold feature labels and practical commands. Never invent release badges, results or installation commands.
- Explain in README that this was built for and as the RIT Dubai SWEN course final project. Do not imply official university deployment or endorsement.
- Prototype accounts are an explicit course requirement (CON-002). Implement actual credential verification, server-side sessions and permissions; label the login simulation and never request university passwords.
- Enforce permissions and public/private field separation on the server. UI hiding is not authorization.
- Keep business changes, append-only history and retry handling within one atomic storage boundary. Verify concurrent approvals, repeated requests, lost responses and storage failures.
- Never silently introduce cancellation, expiry or withdrawal policy for approved claims. Track M3 Q-12 and Q-13 as unresolved policy decisions.
- Keep all user-facing text in a language resource file. Preserve keyboard operation, responsive layouts and the baseline accessibility requirements.
- No sub-agents unless explicitly requested by the user or another applicable instruction.

Research and the proposed five-iteration plan are in docs/design-research.md. The architecture is a recommendation pending implementation validation, not an achieved result.
