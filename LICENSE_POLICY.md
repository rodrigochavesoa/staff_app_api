# License Policy

STAFF App API is distributed under the Apache License 2.0.

This policy explains how maintainers should evaluate third-party code,
dependencies, documentation, data files and generated assets before they are
included in the public package.

This document is project policy, not legal advice. When license risk is unclear,
maintainers should pause the change and request a dedicated legal or open source
license review.

## Project License

- Primary project license: Apache-2.0.
- License file: [LICENSE](LICENSE).
- Contributions are accepted under the same license unless explicitly stated
  and approved by maintainers.

Apache-2.0 is permissive and includes patent license language. It allows use,
modification and distribution, including commercial use, provided the license
terms and notices are respected.

## Allowed Dependency Licenses

The following licenses are generally allowed:

- Apache-2.0;
- MIT;
- BSD-2-Clause;
- BSD-3-Clause;
- ISC;
- Zlib;
- Unicode-DFS or Unicode-3.0 for standard Unicode data, when applicable.

## Review Required

The following require maintainer review before inclusion:

- MPL-2.0;
- EPL-2.0;
- LGPL;
- CC-BY;
- CC0;
- public datasets;
- generated assets;
- documentation copied from external sources;
- model outputs or AI-generated text intended for bundled distribution.

Review should confirm:

- license compatibility with Apache-2.0 distribution;
- attribution requirements;
- whether the asset can be redistributed;
- whether a `NOTICE` file is required;
- whether source, generated or binary form changes the obligation.

## Blocked Licenses and Materials

Do not introduce:

- GPL-2.0 or GPL-3.0 dependencies;
- AGPL-3.0 dependencies;
- SSPL;
- Commons Clause;
- non-commercial licenses;
- no-derivatives licenses;
- proprietary code or assets without written permission;
- copied book/manual content without redistribution rights;
- production data, private health data or customer data.

Exceptions require explicit maintainer approval and documented legal review.

## Bundled Assets and Knowledge Base Content

The public repository may include project-owned or permissively licensed data
needed at runtime, such as:

- `data/csv` exercise metadata;
- `data/json` deterministic running templates.

Knowledge base chunks must be:

- original project content;
- public domain;
- permissively licensed;
- or covered by explicit redistribution permission.

Do not bundle long excerpts from commercial books, proprietary manuals,
restricted PDFs or private datasets.

## NOTICE File Policy

If bundled third-party material requires attribution notices, maintainers should
add a root `NOTICE` file before release.

Do not add a `NOTICE` file as decoration. Add it only when there are actual
notices or attributions that must travel with the distribution.

## Dependency Review Checklist

Before adding a dependency:

1. Check its license.
2. Confirm it is actively maintained or small enough to audit.
3. Run `go mod tidy`.
4. Run vulnerability and static checks.
5. Update documentation if the dependency affects runtime, security or
   deployment.
6. Record any license concern in the PR.

Recommended commands:

```bash
go list -m all
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
go run github.com/securego/gosec/v2/cmd/gosec@v2.22.10 ./...
```

## SBOM

Before a public release, maintainers should generate and archive an SBOM for
the source package and Docker image when practical.

Acceptable tools may include:

- Syft;
- GoReleaser SBOM support;
- GitHub dependency graph and dependency review.

The SBOM should not include local-only materials excluded from the public
package, such as `docs/`, `api/`, `.env`, databases, logs, backups or uploads.

## Contributor Obligations

By submitting a contribution, contributors confirm that:

- they have the right to submit the work;
- the contribution can be licensed under Apache-2.0;
- no secrets, private data or restricted content are included;
- new dependencies comply with this policy or are clearly flagged for review.
