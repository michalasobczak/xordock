# Manifest

xordock's engineering principles. They apply to every spec in `specs/`. A spec that breaks any of the principles must say so explicitly and justify it in the "Additional Information" section.

The basis is the Engineering essentials from https://workwithrondo.com/#builders (E1–E6). Concrete project decisions (stack, environments, deploy) are added here once they are made.

## E1. Design and technology

**M1. Simplicity.** Proven patterns, clear responsibilities, only the abstractions that are needed.
**M2. No duplication.** Shared logic lives in one place; any exception is justified in the spec.
**M3. Stable technologies and minimal dependencies.** Every library and every additional service (queue, NoSQL, etc.) requires justification.
**Stack:** Go 1.27, no Go dependencies outside the standard library; cgo to `Security.framework` (`SecRandomCopyBytes`) and libfuse3 from macFUSE 5.5 (FSKit backend, installed by the user, not bundled). macOS 26+ (tested on 27), Apple Silicon. WebDAV, NFS and a custom FSKit extension rejected (S002, S004).

**M24. Plaintext only in RAM.** xordock code never writes plaintext content or file names to disk or to logs. Buffers holding content are wiped (`split.Wipe`) after use.
**M25. Honest claims.** Every security message and document states which mask source we use; the CSPRNG is never presented as resistant to P = NP.
**M26. One share, one checksum.** A checksum covers only its own share. We never write to one medium anything computed from the contents of the other.

## E2. Contracts and data

**M4. Versioned API contracts** with compatibility tests.
**M5. Idempotency** of repeated operations.
**M6. Data integrity:** transactions, constraints, versioned and tested migrations.
**M7. Input is untrusted:** validation of type, format, size, range and permissions.

## E3. Access and audit

**M8. Identity verification:** secure passwords, 2FA for administrators, no account enumeration.
**M9. Least privilege** checked on every access to a resource.
**M10. Change audit:** who, when, what (without secrets); four-eyes principle where required.

## E4. System protection

**M11. Secrets outside code, frontend and logs;** separate per environment, with rotation.
**M12. HTTPS, security headers, encryption of data at rest** according to sensitivity.
**M13. Isolation of uploaded files.**
**M14. Restricted access to environments:** no public SSH, only the ports that are needed.

## E5. Deployment and restore

**M15. Code, specs, migrations and configuration (without secrets) in Git;** regular push to a private remote repository.
**M16. Containers** (e.g. Docker Compose).
**M17. Deploy via CI:** build, tests, control gates, deployment; no bypassing of failures.
**M18. Separate environments and a rollback plan** before every deployment.
**M19. Backups** with tested restore.

## E6. Tests and CDD context

**M20. Tests are mandatory;** we do not change expected results to make a test pass.
**M21. We fix causes, not symptoms.**
**M22. AI context revalidation** after every cycle: context is rebuilt from the specs, CHANGELOG, README and tests.

## Language

**M23.** Artifacts in English; communication with the owner in Polish.
