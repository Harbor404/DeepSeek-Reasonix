---
owner: @esengine
backup: @SivanCola
status: active
reviewed: 2026-09-30
---

# Code signing policy

DRAFT. Not in force until the owner resolves every `TO CONFIRM` marker and the Windows signing provider named in section 1 is true. Do not link this page from the README before then.

Scope: Windows and macOS binaries of Reasonix Studio (`studio-v2.*` releases) published from `esengine/DeepSeek-Reasonix`.
Requirements source: <https://signpath.org/terms> (SignPath Foundation OSS terms). Requirements that page does not state are marked `UNVERIFIED`.

## 1. Current signing status

| ID | Fact |
| --- | --- |
| S1 | Windows Authenticode signing of Studio releases uses a Certum certificate through Certum SimplySign cloud signing, when the repository variable `STUDIO_SIGNING_ENABLED` is `true`. |
| S2 | Windows releases are NOT signed through SignPath. The SignPath Foundation certificate is not in use for any release. |
| S3 | macOS packages are signed with an Apple Developer ID and notarized. Linux packages carry a detached minisign signature. |
| S4 | Every package carries a `.minisig` signature and the release carries `SHA256SUMS`. |
| S5 | This page becomes the SignPath Foundation code signing policy only after the Windows release workflow signs through SignPath and section 1 is rewritten to say so. |

## 2. What is signed

| ID | Rule |
| --- | --- |
| P1 | Only binaries built from this repository's own source by `.github/workflows/release-studio.yml` on the `studio` branch MAY be signed. |
| P2 | The signed set is the Studio executables, the installer that packages them, and the bundled `reasonix` CLI archives built in the same run. |
| P3 | Third-party binaries bundled unmodified keep their upstream signatures. They MUST NOT be re-signed with the project certificate. |
| P4 | The signing contract `.signpath/contracts/release-signing.yml` declares the allowed branch, build definitions and the files whose fingerprint the release checks before any signing job runs. |
| P5 | Software containing malware, or features designed to identify or exploit security vulnerabilities in other software, MUST NOT be signed. |

## 3. Build origin and integrity

| ID | Rule |
| --- | --- |
| B1 | Signing runs only inside the GitHub Actions release workflow, started by a `studio-v*` tag or a manual dispatch of that workflow. |
| B2 | The workflow builds the unsigned Windows bundle first. The signing jobs refuse a bundle whose executable set differs from the declared list or whose digest changed after the build. |
| B3 | The signing jobs run in the `studio-release` environment and share the concurrency group `certum-signing`. |
| B4 | Reproducible builds: UNVERIFIED. No claim is made that two builds of one commit are byte-identical. |

## 4. Team roles

Only one role holder is known. Everyone else MUST be added by the owner; none is assumed.

| Role | Members | Responsibility |
| --- | --- | --- |
| Author (committer) | @esengine | Trusted to change source code without a second review. TO CONFIRM: whether any other account has write access and belongs here. |
| Reviewer | @esengine | Reviews every change from a non-committer before it is merged. TO CONFIRM: whether @SivanCola reviews as backup. |
| Approver | @esengine | Approves each signing request, by dispatching the release and approving the `studio-release` environment. TO CONFIRM: approvers must be listed by name; add a second person if one exists. |

## 5. Privacy

| ID | Statement |
| --- | --- |
| V1 | Reasonix runs locally. Prompts, files and session content are sent only to the model providers the user configures. |
| V2 | Anonymous, content-free telemetry (random install ID, version, OS, architecture, surface, fixed-bucket counters) goes to `https://crash.reasonix.io`. Details and opt-out for the CLI are in `docs/GUIDE.md`, section "CLI telemetry". |
| V3 | TO CONFIRM: what the Studio desktop app itself sends, whether it is disclosed during installation, and whether the installer offers a way to disable it. The SignPath terms require all three for software that sends user data to systems the user did not specify. Nothing here is verified for Studio. |

## 6. Attribution line

Use this line only after S2 stops being true:

> Free code signing on Windows provided by [SignPath.io](https://signpath.io), certificate by [SignPath Foundation](https://signpath.org)

## 7. Enforcement

| ID | Rule |
| --- | --- |
| E1 | `go run ./cmd/signpath-contract validate` fails a release workflow that reaches signing credentials without being declared in the contract. |
| E2 | Changes to this page, `.signpath/` and the release workflow require review by the CODEOWNERS listed in `.github/CODEOWNERS`. |
