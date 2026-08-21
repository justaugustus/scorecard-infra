# Security Policies and Procedures

The OpenSSF Scorecard infra maintainers take all security issues seriously.
Thank you for improving the security of scorecard-infra.
We appreciate your efforts and responsible disclosure
and will make every effort to acknowledge your contributions.

This repository holds OpenSSF Scorecard's hosted infrastructure: the batch
scanning pipeline behind the weekly public scan of 1M+ repositories (`cron/`)
and a self-hostable results API server (`cmd/scorecard-api`). Reports against
either are in scope, as are the scan inventories and deployment manifests.

Vulnerabilities in the Scorecard **engine** itself — checks, probes, scoring, or
output formats — belong to
[`ossf/scorecard`](https://github.com/ossf/scorecard/security/policy). If you are
unsure which applies, report it here and we will route it.

## Reporting a Vulnerability

Report security vulnerabilities using
[GitHub's private vulnerability reporting](https://github.com/ossf/scorecard-infra/security/advisories/new).

Please include the following details in your report:

- A description of the vulnerability
- Steps to reproduce the issue
- Affected versions
- Any known mitigations

**Please do not report security vulnerabilities through public GitHub issues.**

## Vulnerability Management Process

When a vulnerability is reported, the maintainers will:

1. **Acknowledge** the report within three (3) business days.
2. **Investigate** the issue, confirm the vulnerability,
   and determine affected versions.
3. **Provide a detailed response** within an additional three (3) business days,
   including an assessment and planned timeline for a fix.
4. **Audit** the codebase for similar issues.
5. **Prepare fixes** for all maintained releases and coordinate disclosure.

## Disclosure Policy

We follow a coordinated disclosure process:

- Reporters will be kept informed of progress throughout the process.
- Fixes will be prepared and tested before any public disclosure.
- Credit will be given to reporters in release notes
  (unless anonymity is requested).

## Escalation

If you do not receive a timely response via GitHub,
or if you are unable to use the private vulnerability reporting feature,
please reach out to the [Scorecard Steering Committee](mailto:scorecard-steering@lists.openssf.org).

## Suggestions for Improvement

If you have suggestions for how this security process could be improved,
please submit a pull request or open an issue.
