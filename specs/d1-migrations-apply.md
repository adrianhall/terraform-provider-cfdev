# Terraform resource for applying D1 migrations

## Overview

### Problem

When Terraform provisions a Cloudflare D1 database and Wrangler (or another Cloudflare
CLI) is used to apply its migrations, applying the migrations remains a separate manual
step. This makes a complete Terraform deployment difficult to reproduce in CI/CD and
means that the database schema is not represented by the Terraform dependency graph.

### Proposed solution

Create and publish the `adrianhall/cfdev` Terraform provider with a
`cfdev_d1_migration` resource. The resource applies pending SQL migration files to a
Cloudflare D1 database and records successful migrations in the same migration table
used by Wrangler/cf by default. Terraform can therefore manage the database and its
schema in one configuration.

The resource is an orchestration resource, not a database resource. The existing
`cloudflare_d1_database` resource remains responsible for creating, updating, and
destroying the database.

### Goals

- Apply pending `.sql` migration files during `terraform apply`.
- Make migration application deterministic: select files, sort them with Wrangler's
  numeric-prefix path ordering, and apply them in that order.
- Interoperate with Wrangler/cf through the compatible default migration table.
- Detect changes to migration files during planning, including a newly added file.
- Expose enough state for refresh/read to detect whether the managed migration set has
  drifted from the database.
- Make the resource usable in automated, non-interactive CI/CD environments.
- Document permissions, setup, lifecycle behavior, compatibility, and operational
  recovery.

### Non-goals

- Creating, updating, or destroying a D1 database.
- Automatically generating migration files.
- Supporting down migrations or rollback by reversing SQL files.
- Replacing Wrangler/cf for local development or ad hoc SQL execution.
- Applying arbitrary SQL supplied directly in Terraform configuration.
- Providing down migrations. Wrangler-style rollback is transactional rollback of the
  currently executing migration, not execution of a reverse/down migration.

## Example configuration

```hcl
terraform {
  required_version = ">= 1.10.0"

  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.22"
    }
    dotenv = {
      source  = "jrhouston/dotenv"
      version = "~> 1.0"
    }
    cfdev = {
      source  = "adrianhall/cfdev"
      version = "~> 1.0"
    }
  }
}

# All demos in this repository share the root .env file.
data "dotenv" "config" {
  filename = "${path.module}/../.env"
}

locals {
  cloudflare_api_token  = sensitive(data.dotenv.config.env["CLOUDFLARE_API_TOKEN"])
  cloudflare_account_id = data.dotenv.config.env["CLOUDFLARE_ACCOUNT_ID"]
  database_name         = "todo-db"
}

provider "cloudflare" {
  api_token = local.cloudflare_api_token
}

provider "cfdev" {
  api_token = local.cloudflare_api_token
}

resource "cloudflare_d1_database" "demo" {
  account_id = local.cloudflare_account_id
  name       = local.database_name
  read_replication = {
    mode = "disabled"
  }
}

resource "cfdev_d1_migration" "demo_migrations" {
  account_id        = local.cloudflare_account_id
  name              = local.database_name
  migrations_dir    = "${path.module}/migrations"
  migrations_pattern = "${path.module}/migrations/0*.sql"
  migrations_table   = "tf_migrations"

  depends_on = [cloudflare_d1_database.demo]
}
```

## Resource contract

### Resource address

- Resource: `cfdev_d1_migration`.
- Provider: `cfdev`, published initially as `adrianhall/cfdev` on the public Terraform
  Registry. The repository must be named `terraform-provider-cfdev` so the Registry can
  discover it.

### Arguments

| Argument | Type | Required | Default | Description |
| --- | --- | ---: | --- | --- |
| `account_id` | string | yes | — | Cloudflare account containing the database. Must be non-empty. |
| `name` | string | yes | — | D1 database name. |
| `migrations_dir` | string | yes | — | Path to the migration directory, resolved by Terraform from the provider process working directory. Documentation must recommend `${path.module}`-relative paths. |
| `migrations_pattern` | string | no | `${migrations_dir}/*.sql` | Wrangler-compatible glob, relative to the Wrangler/project root. If set, it must start with `${migrations_dir}/`; matched migration names are stored relative to `migrations_dir`. |
| `migrations_table` | string | no | `d1_migrations` | Table used to record applied migrations. The default and schema match Wrangler 4.151.0. |

The provider has `cloudflare_api_token` and an optional `cloudflare_api_endpoint`
configuration. The latter defaults to the production Cloudflare API and supports
alternate compatible endpoints. API tokens should be supplied through provider
configuration or the documented Cloudflare environment variables, never stored in source
or migration files.

### File discovery and identity

File discovery is deliberately matched to the same file discovery process that `wrangler` and `cf` CLI tools use.

1. Validate that `migrations_dir` exists and is a directory during plan/apply.
2. Select regular files using the default `${migrations_dir}/*.sql` pattern or a custom
   `migrations_pattern`. Custom patterns may opt into nested paths, but must start with
   `${migrations_dir}/`; directory names are never stored in the database identifier.
3. Normalize paths to `/` separators and sort matched relative paths with Wrangler's
   comparator: compare the leading integer before `_` in each path segment numerically;
   numeric segments sort before nonnumeric segments; otherwise compare segments by
   byte-wise lexical order. Compare path depth only after shared segments compare equal.
   For example, `2_second.sql` sorts before `10_tenth.sql`, which sorts before
   `alpha.sql`; a plain ASCII lexical sort is not compatible.
4. Treat the normalized relative path (not just its basename) as the migration
   identifier, consistent with Wrangler/cf.
5. Read file contents as UTF-8 SQL. Fail with the path and a useful diagnostic if a
   file cannot be read or decoded.
6. Compute a deterministic hash over the selected normalized relative paths and
   contents, including the pattern, table name, and relevant configuration. The hash
   must change when a file is added, removed, renamed, or edited. The exact
   canonicalization must be documented so it is stable across operating systems.

A migration file is pending when its identifier is absent from the configured migration
table. Applied migration identifiers must not be silently re-executed when the file
contents later change. A changed applied file is a migration-authoring error and must
produce a clear diagnostic (or a documented drift diagnostic), rather than applying a
second migration with the same identifier.

### Wrangler-compatible migration table

The v1 compatibility target is Wrangler **4.151.0**. Investigation of its installed
implementation and a local `wrangler d1 migrations apply --local` execution establishes
the default table name and schema below:

```sql
CREATE TABLE IF NOT EXISTS "d1_migrations" (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT UNIQUE,
  applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
);
```

The resource must create this exact schema (with the configured table name safely
SQLite-identifier quoted) if it is missing. The table's behavior is material to
compatibility:

- `id` is the application order and is read with `ORDER BY id`.
- `name` is the Wrangler-normalized migration path relative to `migrations_dir`, and its
  SQLite `UNIQUE` constraint prevents applying an identifier twice.
- `applied_at` is assigned by SQLite/D1 as a `CURRENT_TIMESTAMP` text value; Terraform
  must not write its own timestamp.
- Wrangler appends `INSERT INTO <table> (name) VALUES ('<escaped identifier>')` to each
  migration's SQL and executes that combined statement. The provider must use the same
  per-migration execution semantics and identifier escaping.
- The schema has SQLite's implicit unique index on `name`; it does not store a migration
  content checksum. The provider's directory hash and changed-file diagnostics are
  Terraform-side safeguards, not fields to add to this shared table.

For a custom `migrations_table`, v1 uses this same schema. Do not add provider-only
columns to a table intended to interoperate with Wrangler.

### Create and update

- Read the migration table and determine the pending files.
- Apply pending migrations in sorted order.
- Record each migration only after its SQL succeeds.
- Stop on the first failed migration and return an error that identifies the file and
  preserves the provider/API error. Do not mark a failed migration as applied.
- If no migrations are pending, succeed without executing SQL.
- Update Terraform state with the database identity, configured table, discovered
  migration-set hash, and the applied migration identifiers needed for refresh.
- The resource must not create a duplicate table if the configured table already exists.
  It should use the existing table, with schema compatibility checks where possible.

The resource should ensure that database creation is complete before migration
application when Terraform dependencies are present. The example therefore includes an
explicit `depends_on`; users may also reference a database attribute if the upstream
resource exposes a suitable dependency.

### Read and drift detection

Read should query the configured migration table and refresh state. It must:

- report the resource as existing while the target database and migration table are
  reachable;
- detect missing or inaccessible databases with an actionable diagnostic;
- detect applied migration identifiers that are no longer present in the configured
  directory or whose content hash differs, without attempting destructive repair; and
- cause Terraform to plan application of newly discovered pending files.

Read must not mutate schema or apply SQL. If the migration table is absent, the
provider must return a clear error rather than interpreting all files as safely pending
unless the implementation has explicitly created and initialized that table as part of
resource creation.

### Delete

Delete is a no-op. It must remove the Terraform resource from state without deleting
the migration table, migration records, or user data. Database destruction remains the
responsibility of `cloudflare_d1_database`.

### Atomicity, rollback, concurrency, and failure recovery

The v1 resource must match Wrangler 4.151.0's migration execution semantics. Wrangler
performs the following for each pending migration, in order:

1. It initializes the migration table with `CREATE TABLE IF NOT EXISTS`.
2. It reads applied `name` values and computes pending migration paths.
3. It executes each pending migration separately, in migration order, by appending an
   `INSERT INTO <table> (name) VALUES ('<escaped path>')` statement to that migration's
   SQL and sending the combined SQL as one D1 execution request.
4. It stops after the first failed migration. Wrangler's D1 remote execution contract
   rolls back the failed migration's entire execution, including its bookkeeping
   insert, while leaving earlier successful migrations applied. Wrangler reports this
   as: the failed migration is rolled back and the previous successful migration
   remains applied.

This is transactional rollback of the current migration, not a down-migration system.
It does not undo migrations that completed in earlier requests, and it does not provide
an application-level transaction spanning all pending migrations. A provider retry
therefore re-reads the table and retries the failed/unrecorded migration only. The
provider must preserve this behavior and must not mark a migration applied unless the
combined execution succeeds.

For local execution, Wrangler uses D1's batch API for the statements produced by its SQL
splitter; for remote migration application, Wrangler submits each migration as a
separate D1 execution/import request. The provider's production behavior should follow
the remote contract: one migration per request, SQL and bookkeeping insert in the same
request, and no attempt to wrap all migrations in one cross-migration transaction.

The provider must document that two concurrent applies targeting the same database are
unsupported unless the API/table strategy provides an atomic lock. It should minimize
race conditions by re-reading migration state immediately before execution and should
return a clear error if a duplicate migration is encountered. A failed apply must be
safe to retry after the operator has confirmed the database's actual migration state.
The provider must never issue down migrations or automatically reverse a previously
successful migration.

## Project layout

Use a standalone Go Terraform provider repository named `terraform-provider-cfdev`,
with the following layout. This follows the structure used by `jrhouston/dotenv`, while
using the modern Terraform Plugin Framework rather than copying the older SDK version
of that provider:

```text
.
├── main.go                         # provider server entry point
├── go.mod
├── go.sum
├── GNUmakefile                     # test/testacc/docs convenience targets
├── .goreleaser.yml                 # Registry-compatible release artifacts
├── terraform-registry-manifest.json
├── internal/
│   ├── provider/                   # Terraform schema, resource, lifecycle
│   │   ├── provider.go
│   │   ├── d1_migration_resource.go
│   │   └── d1_migration_resource_test.go
│   ├── d1/                        # Cloudflare D1 client and migration-table logic
│   │   ├── client.go
│   │   ├── migrations.go
│   │   └── migrations_test.go
│   └── files/                     # discovery, ordering, hashing, validation
│       ├── migrations.go
│       └── migrations_test.go
├── examples/
│   └── resources/cfdev_d1_migration/
│       ├── resource.tf
│       └── migrations/
│           └── 001_initial.sql
├── docs/
│   ├── index.md
│   └── resources/d1_migration.md
├── templates/                     # generated Terraform Registry docs, if used
├── examples_test/                 # optional acceptance-fixture support
├── tools/                         # pinned documentation/tool dependencies, if needed
├── .github/workflows/             # lint, unit, acceptance, release
├── CHANGELOG.md
├── LICENSE
└── README.md
```

Keep Cloudflare API code behind a small interface so file processing and resource
lifecycle can be unit-tested without network access. Do not put credentials, generated
state, or real migration data in the repository.

## Languages and implementation choices

- **Go** is the implementation language, using the Terraform Plugin Framework and the
  provider protocol supported by Terraform >= 1.10.0.
- Use the maintained Cloudflare Go client where it supports the required D1 operations;
  otherwise isolate a small, typed HTTP client for the missing operations.
- The provider source address is `registry.terraform.io/adrianhall/cfdev`, and the Go
  module/repository should use `github.com/adrianhall/terraform-provider-cfdev`.
- The provider binary must be served under the Terraform Registry name `cfdev`, with
  the executable name `terraform-provider-cfdev_v<version>` in release archives.
- Use Go's standard filesystem, hashing, sorting, and UTF-8 facilities. Avoid shelling
  out to Wrangler or depending on a locally installed Node.js/pnpm runtime.
- Use HCL for Terraform examples and Markdown for user documentation.
- Pin direct dependencies and commit `go.sum`. Run `go vet`, formatting, and a
  reproducible lint configuration in CI.

## Prerequisites and permissions

Users need:

1. Terraform >= 1.10.0.
2. A Cloudflare account and an existing D1 database, or the official
   `cloudflare_d1_database` resource in the same configuration.
3. A Cloudflare API token with the least privileges required to execute D1 SQL and
   read/write the migration table. The exact permission names must be documented after
   verifying the current Cloudflare API permission model.
4. A migration directory committed with the Terraform configuration and readable by
   the Terraform process.
5. Network access from the Terraform runner to the configured Cloudflare API endpoint.
6. The provider installed from `registry.terraform.io/adrianhall/cfdev`.
7. For provider development and releases: Go, Git, Terraform (for example formatting,
   validation, and local examples), GoReleaser, and a GPG signing key registered with
   the Terraform Registry. Ordinary unit-test CI must not require Cloudflare
   credentials; acceptance tests do.

The provider must support non-interactive authentication and must redact tokens and SQL
parameters from diagnostics and logs. It must not require Wrangler, Node.js, pnpm, or
Cloudflare's CLI.

## Testing plan

### Unit tests

Test the filesystem and migration algorithm without network access:

- missing/non-directory `migrations_dir`;
- default and custom migration patterns, including the required directory prefix;
- regular files versus directories and unsupported file types;
- Wrangler numeric-prefix ordering, including numeric-looking names, mixed case, and
  nested path segments;
- deterministic hashes across repeated runs;
- hash changes for add/remove/rename/content/pattern/table changes;
- unreadable and invalid UTF-8 files;
- empty directories and no-op applies;
- pending, applied, missing, and changed migration identifiers;
- default and custom migration table names; and
- validation of required arguments and invalid paths.

Test the D1 client with an `httptest` server or injected interface:

- successful query and execution;
- authentication and HTTP errors;
- malformed API responses;
- SQL execution failure;
- migration-table creation, the exact Wrangler 4.151.0 schema, and schema mismatch
  behavior; and
- pagination/retry behavior if the API requires it.

### Provider/resource tests

Use Terraform Plugin Framework resource tests to verify plan, create, update, read, and
delete semantics. Assert that:

- create applies only pending files and records successful files;
- update applies a newly added file and does not reapply an existing file;
- a failed file is not recorded as applied;
- read refreshes state and reports drift clearly;
- delete makes no remote mutation; and
- sensitive provider configuration is not emitted in state or diagnostics.

### Acceptance tests

Behind an explicit environment flag, run acceptance tests against a dedicated disposable
Cloudflare account/database. Cover real D1 SQL, Wrangler-compatible table behavior,
multiple applies, a new migration after the first apply, and failure/retry behavior.
Acceptance tests must clean up test databases and never run against a production account.

### Quality gates

CI should run formatting checks, `go vet`, static analysis/linting, unit tests with race
detection where practical, provider documentation generation/checks, and acceptance tests
only when credentials are present. Release builds must be reproducible for supported
platforms and include checksums/signatures according to the provider registry process.

## Documentation requirements

Documentation must include:

- provider installation and final source address;
- provider authentication, `cloudflare_api_endpoint`, and minimum Cloudflare
  permissions;
- complete resource argument/reference documentation;
- a minimal working example with a correctly located `migrations/` directory;
- filename conventions, Wrangler numeric-prefix ordering, `migrations_pattern`
  semantics, hashing, and migration identity rules;
- compatibility with Wrangler 4.151.0/cf and the default `d1_migrations` table,
  including its exact schema and per-migration insert behavior;
- how to use a custom table and its required schema;
- dependency ordering with `cloudflare_d1_database`;
- plan/apply/read/delete behavior, including delete being a no-op;
- handling of SQL failures, Wrangler-compatible per-migration transaction rollback,
  changed applied files, partial applies, concurrent applies, unknown commit status,
  and recovery;
- CI/CD and secret-management guidance;
- limitations: no down migrations, no automatic rollback, and no recursive discovery
  unless added later; and
- an upgrade/changelog policy, support matrix, and security/contact process.

Generated Terraform Registry documentation should be checked in or generated in CI,
consistent with the chosen provider publishing workflow. Include an example fixture in
CI so documentation does not drift from the implementation.

## Potential blockers and risks

- **D1 API capability or semantics:** The required remote SQL execution and migration
  table operations must be available through a stable API and supported by the selected
  Go client. API limits, response formats, and statement-size limits may constrain SQL
  files.
- **Migration-table compatibility:** Wrangler/cf may change its table schema or
  migration-discovery semantics. Compatibility is pinned to Wrangler 4.151.0 for v1 and
  must be covered by integration tests before upgrading that baseline.
- **Partial execution:** Wrangler sends each migration's SQL and bookkeeping `INSERT`
  in one D1 execution request, and D1 rolls back that failed request. Earlier successful
  migrations remain applied because there is no transaction spanning all pending
  migrations. A transport failure whose commit status is unknown can still require
  operator reconciliation before retrying.
- **Concurrent applies:** Terraform cannot by itself serialize applies from different
  runners targeting one database. Concurrent migration requests can race on the
  migration-table `UNIQUE` constraint and must be treated as unsupported in v1.
- **Terraform planning limitations:** Terraform may not reliably detect filesystem
  changes when paths are outside the module or are excluded from the execution
  environment. CI must package/checkout migrations consistently.
- **Provider publication:** The provider namespace, registry ownership, signing, and
  release automation must be available before users can consume the example.
- **Permissions and rate limits:** Incorrect API-token permissions and D1 API limits can
  look like migration failures; diagnostics must distinguish authentication,
  authorization, throttling, and SQL errors.
- **Irreversible SQL:** A successful migration cannot generally be undone automatically.
  Operators need backups and a forward-fix procedure.

## Alternatives considered

These are cheaper alternatives, but do not solve the underlying issue of requiring to manage a complete deployment within Terraform:

1. A thin wrapper: `terraform_data` with `local-exec` that runs `wrangler d1 migrations apply` and `wrangler deploy`, triggered by hash inputs.  It's ugly, but works today.

2. Terraform's newer actions feature, if you're on a recent enough version.  If models "run this operation" more honestly than a fake resource does.

The desire is for a coherent, reusable tool.

## Resolved design decisions

The following decisions were supplied after the initial specification review:

1. **Provider identity:** The initial provider source address is
   `registry.terraform.io/adrianhall/cfdev`, hosted in the public GitHub repository
   `adrianhall/terraform-provider-cfdev`. The provider's local name and binary name are
   `cfdev` / `terraform-provider-cfdev`.

2. **Wrangler/cf compatibility:** v1 targets Wrangler 4.151.0. Its migration table is
   `d1_migrations`, with the exact three-column schema and migration-insert behavior
   documented in [Wrangler-compatible migration table](#wrangler-compatible-migration-table).
   Compatibility tests must pin that version; upgrades require an explicit compatibility
   review rather than assuming the schema remains unchanged.

3. **Atomicity and rollback:** The provider adopts Wrangler 4.151.0 semantics. Each
   migration's SQL and its `d1_migrations` bookkeeping insert are sent in one D1
   execution request. D1 rolls back that failed request, including the bookkeeping
   insert, if the migration fails; earlier successful migrations remain committed.
   There is no transaction spanning all pending migrations and no down-migration
   rollback. The provider must retry only migrations that are still absent from the
   table, and an unknown transport/commit outcome requires state verification before
   retry.

4. **API endpoint:** The production API must be the default, with an optional
   `cloudflare_api_endpoint` provider configuration value alongside
   `cloudflare_api_token` for alternate environments.

## Release and distribution process

The initial release follows the public Terraform Registry distribution model used by
`jrhouston/dotenv`, adapted to the `adrianhall/cfdev` namespace and the modern provider
implementation:

1. Host the public repository at `github.com/adrianhall/terraform-provider-cfdev`.
   Terraform Registry provider discovery requires the repository name pattern
   `terraform-provider-cfdev` and a public GitHub repository.
2. Set the provider address consistently to `registry.terraform.io/adrianhall/cfdev` in
   `main.go`, examples, documentation, and debug configuration.
3. Use semantic version tags prefixed with `v`, such as `v1.0.0`; do not reuse or mutate
   a released tag.
4. Use GoReleaser from `.goreleaser.yml` to build Registry-compatible ZIP archives for
   supported OS/architecture pairs. Set `CGO_ENABLED=0`, use `-trimpath`, embed
   version/commit metadata, and name binaries `terraform-provider-cfdev_v<version>`.
5. Produce a SHA-256 sums file, its binary GPG signature, and
   `terraform-provider-cfdev_<version>_manifest.json` containing the supported protocol
   version. The manifest should declare protocol `6.0` for the Plugin Framework unless
   the implementation deliberately configures protocol 5.
6. Add a GitHub Actions release workflow triggered by version tags. It should run tests,
   build and sign artifacts with GoReleaser, and publish a GitHub Release. Store the
   GPG private key, passphrase, and GitHub token as repository secrets; never commit
   them.
7. Register the public signing key in the Terraform Registry, publish the
   `adrianhall/cfdev` provider through the Registry UI, and verify Registry ingestion
   from the GitHub release webhook.
8. Keep `README.md`, `CHANGELOG.md`, generated `docs/`, release configuration, and
   checksums aligned. Registry checksums are immutable, so corrections require a new
   version.

This is intentionally modeled on `jrhouston/dotenv`'s repository conventions — Go
modules, `main.go`, `GNUmakefile`, generated docs, GoReleaser, GitHub Releases, signed
checksums, and Registry installation — but updates the implementation and release
metadata for `adrianhall/cfdev`.

## Definition of done

- The provider is published under the final source address and can initialize with the
  example configuration.
- `cfdev_d1_migration` applies sorted pending SQL files, records successful migrations,
  refreshes state, detects meaningful drift, and performs a no-op delete.
- Unit, provider, and gated acceptance tests cover the behaviors listed above.
- Authentication, permissions, compatibility, failure recovery, and limitations are
  documented, including the corrected migrations directory example.
- CI validates code quality, generated documentation, and release artifacts without
  requiring secrets for ordinary pull requests.

## Phased implementation plan

The implementation plan has been decomposed into one task specification per file under
[`specs/tasks/`](tasks/). Begin with [`issue.md`](../issue.md) from the repository root,
after renaming this directory to `terraform-provider-cfdev`. The implementation session
must treat that renamed repository as the canonical working directory and use the
provider identity `adrianhall/cfdev` throughout. Each task is a separately reviewable
unit of work. Every implementation task follows the same delivery loop:

1. Validate the current repository state, branch, outstanding changes, tool versions,
   and the previous step's acceptance criteria.
2. Write failing tests first for the seam being implemented (TDD).
3. Implement the smallest coherent change.
4. Run the seam tests and validate that they pass green.
5. Run formatting, lint/static checks, validation, the complete test suite, and coverage;
   target at least **80% line coverage** for provider code, with any exclusions recorded.
6. Review the diff and make a conventional commit whose scope identifies the seam.

A step is not complete until its commit, test evidence, coverage result, and any known
follow-up risk are recorded in the session summary. Do not combine unrelated phases in
one commit.

### Phase 1 — Repository and provider foundation

1. **Repository rename and baseline inventory** — Confirm the directory is
   `terraform-provider-cfdev`, inspect the clean/dirty Git state, verify Go/Terraform/
   Wrangler/tool versions, and establish the baseline build and coverage report. Add
   the license, `.gitignore`, `go.mod`, `go.sum`, `README.md`, `CHANGELOG.md`, and
   initial provider metadata. Add tests around provider identity and startup, implement
   the provider entry point, validate all tests and checks, then commit
   `chore(repo): initialize cfdev provider repository`.
2. **Provider configuration and authentication** — Validate the foundation baseline.
   Write tests for provider configuration, required/optional attributes, default
   production endpoint, alternate `cloudflare_api_endpoint`, token redaction, and
   environment-variable authentication. Implement the provider configuration and typed
   Cloudflare client factory, run the complete green validation/coverage loop, and
   commit `feat(provider): add cfdev configuration and authentication`.

### Phase 2 — Deterministic migration model

3. **Migration discovery and path normalization** — Validate provider configuration and
   coverage. Write tests for the default `${migrations_dir}/*.sql` pattern, custom
   patterns, required directory prefix, regular-file filtering, nested paths, `/`
   normalization, and missing/invalid directories. Implement the file discovery seam,
   run the green validation/coverage loop, and commit
   `feat(files): implement migration discovery`.
4. **Wrangler-compatible ordering and identity** — Validate discovery behavior against
   the pinned Wrangler 4.151.0 behavior. Write tests for numeric-prefix ordering,
   nonnumeric names, mixed case, nested segments, path-depth ties, and relative-path
   migration identifiers. Implement the comparator and identity rules, run the green
   validation/coverage loop, and commit `feat(files): match wrangler migration ordering`.
5. **Migration hashing and changed-file detection** — Validate discovery and ordering
   baselines. Write tests proving stable hashes and changes for additions, removals,
   renames, content edits, pattern changes, table changes, and platform-independent
   path separators. Implement canonical hashing and applied-file drift diagnostics,
   run the green validation/coverage loop, and commit `feat(files): add migration set hashing`.

### Phase 3 — D1 execution and Terraform resource lifecycle

6. **D1 SQL client seam** — Validate provider authentication and the existing test
   doubles. Write HTTP contract tests for authentication, endpoint selection, request
   encoding, response decoding, API errors, rate limits, retries, and redaction. Implement
   the minimal typed D1 client interface, run the green validation/coverage loop, and
   commit `feat(d1): add typed D1 client`.
7. **Migration-table compatibility** — Validate the D1 client seam. Write tests for
   safely quoted table identifiers, creation of the exact Wrangler-compatible schema,
   `ORDER BY id` reads, the unique `name` constraint, `CURRENT_TIMESTAMP`, and schema
   mismatch diagnostics. Implement table initialization and reads, run the green
   validation/coverage loop, and commit `feat(d1): support wrangler migration table`.
8. **Atomic per-migration execution** — Validate table reads and the pinned Wrangler
   semantics. Write tests proving that SQL and the bookkeeping insert are sent in one
   request, failures do not record the migration, previous successful migrations remain
   applied, and retries select only absent migrations. Implement one-request-per-
   migration execution, run the green validation/coverage loop, and commit
   `feat(d1): apply migrations atomically per migration`.
9. **Terraform resource schema and planning** — Validate all lower-level seams and
   coverage. Write Plugin Framework tests for required fields, defaults, plan-time
   filesystem validation, stable state, sensitive values, and migration-set hash
   changes. Implement `cfdev_d1_migration` schema and plan behavior, run the green
   validation/coverage loop, and commit `feat(resource): add migration resource schema`.
10. **Terraform create, update, read, and no-op delete** — Validate the planned resource
    contract. Write lifecycle tests for initial apply, no pending migrations, newly
    added migrations, changed applied files, missing tables/databases, refresh drift,
    partial failure, retry, and no-op delete. Implement lifecycle methods and diagnostics,
    run the green validation/coverage loop, and commit
    `feat(resource): implement d1 migration lifecycle`.
11. **Acceptance and compatibility verification** — Validate unit/resource coverage and
    the dedicated test-account safeguards. Write gated acceptance tests against a
    disposable D1 database covering Wrangler interoperability, multiple applies,
    custom tables, failure/retry, and real API endpoint handling. Implement fixtures,
    cleanup, and CI gating; run green checks and coverage (keeping acceptance tests
    separate from the 80% unit-coverage target), then commit
    `test(acceptance): verify remote d1 migration behavior`.

### Phase 4 — Provider usability and release preparation

12. **Examples, generated provider docs, and local developer tooling** — Validate the
    resource behavior and generated-doc baseline. Write tests/checks that examples use
    `adrianhall/cfdev`, the correct `migrations/` directory, and valid Terraform syntax.
    Implement generated Registry docs, `GNUmakefile`, pinned tool dependencies,
    formatting/validation targets, and a contributor-friendly local workflow. Run the
    green validation/coverage loop and commit `docs(provider): add examples and generated docs`.
13. **Security, failure recovery, and quality gates** — Validate all checks and inspect
    diagnostics for leaked tokens or SQL. Write tests for redaction, malformed API
    responses, unknown commit status guidance, concurrent-apply diagnostics, and
    least-privilege configuration. Implement logging, recovery documentation, static
    analysis, race testing where practical, and the 80% coverage gate. Run the green
    validation/coverage loop and commit `chore(quality): enforce provider quality gates`.
14. **Documentation set and contribution policy scaffolding** — Validate the examples,
    generated docs, and release metadata. Write link/configuration checks for the
    documentation set, then add `CONTRIBUTING.md` stating that contributions are
    accepted through pull requests only and every PR must reference an issue;
    `RELEASING.md` describing tags, signing, GoReleaser, Registry publication, and
    rollback/recovery; `SECURITY.md`; `SUPPORT.md`; `CODE_OF_CONDUCT.md`; and a
    `CHANGELOG.md` policy. Run the green validation/coverage loop and commit
    `docs(repo): add contribution and maintenance documentation`.

### Phase 5 — GitHub automation, release, and public documentation

15. **PR and release CI/CD** — This is the next-to-last implementation step. Validate
    branch protection expectations, repository secrets availability, release signing
    prerequisites, and the current workflow/tool versions. Write workflow/configuration
    tests or dry-run checks for pull requests, pushes, tags, unit tests, lint, format,
    Terraform example validation, coverage >=80%, acceptance-test gating, generated-doc
    drift, GoReleaser artifacts, checksums, signatures, and Terraform Registry manifest
    contents. Implement GitHub Actions workflows for PR validation and tagged releases,
    use least-privilege permissions, publish signed GitHub Release artifacts, and wire
    Registry ingestion. Run the complete green validation/coverage loop plus release
    dry-runs, then commit `ci(github): add pull request and release pipelines`.
16. **Documentation** — This is the final implementation step. Validate the released
    provider version, public Registry/GitHub links, examples, and generated API docs.
    Write documentation acceptance checks for navigation, code samples, links, and
    versioned release content. Build and publish a GitHub Pages site at
    `adrianhall.github.io/terraform-provider-cfdev/` (or the final repository Pages
    URL) using the common five-phase documentation structure:

    1. **Just get it done** — installation, API token, minimal Terraform configuration,
       first migration, and the shortest path to a successful apply.
    2. **Beginner** — Terraform concepts, D1 setup, migration naming, directory layout,
       plans, applies, refresh, and safe local development.
    3. **Intermediate** — CI/CD, custom migration patterns/tables, Wrangler
       interoperability, dependencies, drift detection, failure recovery, and team
       workflows.
    4. **Advanced** — D1 transaction semantics, partial application, unknown commit
       status, concurrency, security, endpoint overrides, acceptance testing, and
       provider internals.
    5. **API docs** — provider configuration, `cfdev_d1_migration` arguments, schema,
       lifecycle behavior, examples, compatibility matrix, and generated reference
       material.

    The site must include usage examples and links to the Registry, source repository,
    issue tracker, releases, security policy, and contribution guide. Add a GitHub Pages
    build/publish job to the release workflow so every new provider version regenerates,
    validates, and publishes the site with versioned content. Run the final documentation
    build, link checks, full formatting/lint/tests, and coverage report; then commit
    `docs(site): publish cfdev provider documentation`.

### Required implementation artifacts

By the end of the plan, the repository should contain at least:

- `README.md`
- `CONTRIBUTING.md` — PR contributions only; every contribution must reference an issue
- `RELEASING.md`
- `CHANGELOG.md`
- `SECURITY.md`
- `SUPPORT.md`
- `CODE_OF_CONDUCT.md`
- `LICENSE`
- generated Terraform Registry documentation under `docs/`
- runnable Terraform examples under `examples/`
- GoReleaser, Terraform Registry manifest, Makefile, and GitHub Actions configuration
- GitHub Pages source/configuration and a release-triggered site deployment

The implementation session should stop and report rather than silently weaken a gate.
Any coverage below 80%, undocumented acceptance-test exclusions, failed generated-doc
checks, unsigned artifacts, or an unavailable release credential is a blocking result
that must be resolved or explicitly escalated before the next phase.
