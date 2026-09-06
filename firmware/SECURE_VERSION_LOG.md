# Anti-rollback secure version ledger

Every change to [`SECURE_VERSION`](SECURE_VERSION) gets a row here, written by
`scripts/bump-secure-version.sh`. Appended rows only — never edit a row that
shipped, because the value it records is already burned into node eFuses.

Policy, mechanics and the one-way eFuse warning live in
`docs/notes/anti-rollback-secure-version.md`.

| Date | From | To | Reason | Author | Commit |
|------|------|----|--------|--------|--------|
| 2026-09-06 | 0 | 1 | Wire anti-rollback secure_version into the release process (spaxel-497aee78). First non-zero value: until now every image was built with secure_version 0, so the eFuse check could never reject anything. | jedarden | 6803d885 |
