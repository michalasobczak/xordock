# xordock — instructions for the assistant

We work according to the methodology described at https://workwithrondo.com (Rondo + Context-Driven-Development).

- **Questions first.** Before implementing, I ask (Socratic method) until the goal, scope and acceptance criteria are clear. I don't guess requirements. I record the owner's decisions in the spec.
- **Spec before code.** Every change has a spec in `specs/` with a fixed 9-section structure (template: `specs/SZABLON.md`, numbering `SNNN-name.md`).
- **After delivering a change:** tests confirming the control gates, an entry in `CHANGELOG.md` (by date, newest on top), an updated state in `README.md`, spec status "delivered".
- **Security analysis after every round.** When a spec is implemented and the tests pass, I run, unprompted, a security analysis of this and all previous specs: regressions and gaps. Every finding is fixed like a normal change (spec, test reproducing the problem, CHANGELOG entry). A round ends only after the analysis.
- **Engineering principles** are in `MANIFEST.md` (based on Engineering essentials E1–E6 from workwithrondo.com). They apply to every spec. A deviation must be named and justified in the "Additional Information" section.
- **Reproducibility.** Another modern AI assistant should be able to rebuild the project from `specs/`, `CHANGELOG.md`, `README.md` and the tests.
- **Language:** artifacts in English (code, comments, UI and CLI texts, specs, docs, CHANGELOG, README); conversation with the owner in Polish. Owner's decision of 2026-11-22 (S012).

## Project

- Build: `go build -o bin/xordock ./cmd/xordock`. Tests: `go test -race ./...` and `(cd macos && DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer swift test)` (must pass before every commit). App: `scripts/build-app.sh`.
- E2E test media: the SD card `/Volumes/xordock` or the owner's USB sticks (`/Volumes/xor_primary`, `/Volumes/xor_backup`). On a card we write only under `xordock/<vault-name>`; test vaults are deleted after the tests.
- Security state and open findings: `docs/security/` (latest report).
