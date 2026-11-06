# Cyber Resilience Act: ship-it's commitments

> User decision (L1): **"commit as far as possible, without inventing anything"**. We therefore apply a manufacturer's obligations **voluntarily**, even though ship-it, a non-monetized personal project, is probably out of scope. Nothing here is legal advice.
> Legend: ✅ official text or source · 🟡 secondary.

## 1. What Regulation (EU) 2024/2847 says

| Topic | Content | Status | Source |
|---|---|---|---|
| Scope | Commercial activity; non-commercial activity is out of scope; a *steward* is a legal person that systematically supports OSS intended for commercial use (art. 3(14)) | 🟡 | [OpenSSF stewards playbook](https://policy.openssf.org/CRA/stewards-playbook.html), [openresource.dev](https://openresource.dev/articles/cyber-resilience-act-for-maintainers/) |
| Reporting (art. 14), **in force since Sept. 11, 2026** | **Actively exploited** vulnerability or severe incident: **early warning < 24 h**, notification < 72 h, final report ≤ 14 days after a fix is made available (1 month for a severe incident); through **ENISA's single reporting platform** (`portal.cra-srp.enisa.europa.eu`, online since Sept. 11, 2026) | 🟡 consistent | [cyberresilienceact.eu](https://www.cyberresilienceact.eu/reporting.html), [Crowell](https://www.crowell.com/en/insights/client-alerts/its-live-the-cyber-resilience-act-reporting-is-mandatory-as-of-today-11-september-2026), [Commission](https://digital-strategy.ec.europa.eu/en/policies/cra-reporting) |
| Support period (art. 13(8)) | **At least 5 years** (unless the expected use is shorter); end date announced, at least month and year | 🟡 reproduced text | [craact.eu art. 13](https://www.craact.eu/regulation/cra/article/13/obligations-of-manufacturers) |
| Vulnerability handling (annex I, part II) | During the support period, third-party components included; fixes **without delay** and, where possible, **separate** from functional changes | 🟡 | same |
| Retention | Fixes available for 10 years (or the support period if longer); technical documentation and declaration kept for 10 years | 🟡 | same |
| SBOM | Required at full application (**Dec. 11, 2027**) | 🟡 | [Anchore](https://anchore.com/sbom/eu-cra/) |
| Steward (art. 24) | A documented, verifiable cybersecurity policy, versioned in the repository; cooperation with the authorities; partial reporting; from Dec. 11, 2027 | 🟡 | [art. 24](https://www.european-cyber-resilience-act.com/Cyber_Resilience_Act_Article_24.html) |

Not found: the product class (default or annex III) and the assessment procedure (module A) for a tool like ship-it. ❓ Check in the official text before any commercial offer.

## 2. Commitments adopted (decision of 2026-10-22)

- **Adopted**: E5 SBOM with every release, E6 provenance (attestations) + signing, E7 blocking `govulncheck` in CI.
- **Lightweight version** for the rest: `SECURITY.md` with a private reporting channel; **only the latest version is supported** (common OSS practice).
- **Not adopted for now**: 5-year support, 24 h / 72 h / 14 d runbook, separate fix branches, 10-year archives. To reopen if the project becomes widely public or commercial.

## 3. Commitments considered (detail, for the record)

| # | Commitment | Implementation | Phase |
|---|---|---|---|
| E1 | **Versioned security policy** | `SECURITY.md`: supported versions, private channel (GitHub Private Vulnerability Reporting ❓ to enable), response times | 0-1 |
| E2 | **Announced support period** | Each major version gets security fixes for **5 years** (modeled on art. 13(8)); end date published | 1 |
| E3 | **Separate security fixes** | `release/x.y` branch: security patches published without new features | 1 |
| E4 | **24 h / 72 h / 14 d process** | Internal runbook in `docs/security/`; we reproduce the deadlines even without the obligation (without ENISA notification if not subject to it ❓) | 1 |
| E5 | **SBOM with every release** | SPDX through GoReleaser (Syft), published with the binaries | 1 |
| E6 | **Provenance and signing** | `actions/attest` + cosign; Windows signing (SignPath) as soon as eligible | 1 / 8 |
| E7 | **Dependency vulnerabilities** | Blocking `govulncheck` in CI; Dependabot; dependency budget (ADR-0012) | 1 |
| E8 | **Archives** | Releases and SBOMs kept for 10 years (GitHub Releases + mirror ❓) | 1 |
| E9 | **Security advisories** | GitHub Security Advisories + CVEs when relevant | 1 |
