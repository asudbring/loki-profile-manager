# Security Policy

## Supported versions

Loki Profile Manager is pre-1.0. Security fixes land on `main` and in the next prerelease or stable release. Use the newest published release unless a release note says otherwise.

## Reporting a vulnerability

Report security issues privately through GitHub Security Advisories for this repository when available. If advisories are not available, open a GitHub issue that describes the affected component without including secret values, exploit code for active abuse, or private personal data.

Include:

- Affected version or commit.
- Operating system and architecture.
- Minimal reproduction steps.
- Impact assessment.
- Whether secrets, local files, or synced stores could be exposed or modified.

Do not include real credentials, tokens, secret values, private keys, or private synced-store contents in reports.

## Project security boundaries

Loki is a local CLI. It reads and writes files selected by manifests, can render templates from secret providers, and can activate profile targets with symlinks, copies, merges, and restore operations. Treat manifests and imported skill archives as trusted input from your own store. Loki still validates paths, archives, and unsafe overwrite scenarios, but it is not a sandbox for arbitrary untrusted content.
