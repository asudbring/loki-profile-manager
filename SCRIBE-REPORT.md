# Scribe Report: loki-profile-manager

## Files updated

- `SCRIBE-PLAN.md` — refreshed Scribe inventory, as-coded model, findings, scope, and validation plan for the install-doc pass.
- `README.md` — refreshed project status, install route, first-run route table, quick examples, `--capture-local` guidance, and documentation index.
- `docs/INSTALL.md` — rewrote the install guide into detailed cross-platform install, fresh-machine first-run, existing-machine migration/adoption, safe switch/capture, app smoke, OS-specific troubleshooting, source build, Docker validation, and rollback guidance.
- `docs/USAGE.md` — added first-run workflow summaries and linked the existing-machine safety path; updated switch behavior to include active-profile marker writes.
- `docs/ARCHITECTURE.md` — documented `active_profile.txt`, Windows Known Folder target expansion, and switch/capture flow updates.
- `docs/DEVELOPMENT.md` — added validation rules for install-doc changes and real app/manual switch dogfood.
- `CHANGELOG.md` — added Unreleased documentation and validation notes for the install guide refresh and Loki-owned app/manual switch validation.
- `docs/ai-ops/install.ai.md` — replaced development-checkout-only procedure with an AI-operator install/bootstrap procedure for npm/source installs plus fresh/existing first-run paths.
- `docs/ai-ops/windows-arm64-vm-test.ai.md` — linked the install guide, tightened safety rules, and added optional real-profile app/manual switch validation.
- `docs/test-plans/windows-vm-infisical-tui-smoke.md` — added optional real app/manual switch checks and fail criteria.
- `tests/cross-machine-dogfood-copilot.ai.md` — clarified that the prompt is limited to disposable cross-machine validation and points full installs to `docs/INSTALL.md`.
- `tests/real-dotfile-dogfood-copilot.ai.md` — clarified scope and noted general `--capture-local` usage without changing the narrow prompt behavior.
- `tests/real-dotfile-targeted-restore-consent-copilot.ai.md` — clarified that the prompt is targeted restore only and points install/migration work to `docs/INSTALL.md`.

## Files created

- None.

## Files removed

- None.

## Diagrams

- No new diagrams.
- Existing `docs/ARCHITECTURE.md` switch-flow Mermaid sequence was updated for `--capture-local` and active-profile marker writes.

## Validation

Passed:

- `go test ./...`
- `go vet ./...`
- `go mod verify`
- `go build -trimpath -o /tmp/loki-doc-audit ./cmd/loki`
- `bash -n scripts/install.sh scripts/uninstall.sh scripts/package-release.sh scripts/package-npm.sh scripts/release-local.sh scripts/validate-cross.sh scripts/parallels-windows-admin-probe.sh`
- `node --check npm/bin/loki.js`
- PowerShell parser check for `scripts/install.ps1`, `scripts/uninstall.ps1`, `scripts/windows-installer-smoke.ps1`, `scripts/validate-local.ps1`, and `scripts/windows-onedrive-smoke.ps1`
- `git diff --check`
- Markdown link path check across updated docs, excluding fenced code blocks

## Security and privacy scan summary

- No secret values were added.
- Secret-pattern scan found only existing/intentional placeholder-style command examples:
  - `docs/test-plans/windows-vm-infisical-tui-smoke.md` references assigning `INFISICAL_TOKEN` from `infisical login` without printing it.
  - `docs/USAGE.md` shows `INFISICAL_CLIENT_SECRET = "<value>"` as a placeholder, not a real value.
- Updated docs continue to warn against printing `INFISICAL_CLIENT_SECRET`, `INFISICAL_TOKEN`, or rendered secret values.

## Deferred / out of scope

- No code changes.
- No package release, tag, or npm publish.
- No deletion of VM or local `.dotfiles` backups.
- No real local Mac profile migration or profile switch.
- No new package-manager installers.
- No Azure Key Vault or additional secret-provider docs beyond noting they remain unimplemented.

## Suggested follow-ups

- Add a small disposable fixture store for docs examples if future docs validation should execute `verify <profile>` and `switch <profile> --dry-run` end-to-end without a real profile store.
- Consider a future `loki audit legacy-refs` command to automate the legacy profile-reference scan used during dogfood.
- Consider a future `docs/MIGRATION.md` split if the existing-machine migration guide grows beyond the install guide.
