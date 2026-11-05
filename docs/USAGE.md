# Usage

This guide covers installing PlanTwin, the normal workflow, every command and flag, the configuration reference, CI
integration, and troubleshooting. For the design, see [ARCHITECTURE.md](ARCHITECTURE.md).

## Contents

1. [Concepts](#1-concepts)
2. [Installation](#2-installation)
3. [Workflow at a glance](#3-workflow-at-a-glance)
4. [Set up a repository](#4-set-up-a-repository)
5. [Export statistics from a replica](#5-export-statistics-from-a-replica)
6. [Run a check](#6-run-a-check)
7. [Read the report](#7-read-the-report)
8. [Command reference](#8-command-reference)
9. [Configuration reference](#9-configuration-reference)
10. [Continuous integration](#10-continuous-integration)
11. [Accepting findings](#11-accepting-findings)
12. [Exit codes](#12-exit-codes)
13. [Troubleshooting](#13-troubleshooting)
14. [Operating PlanTwin](#14-operating-plantwin)

---

## 1. Concepts

| Term | Meaning |
|---|---|
| **Bundle** | The signed statistics file exported from a replica. It holds sizes, column statistics and planner settings, and no rows. |
| **Twin** | A throwaway PostgreSQL 18 container used for one side of a comparison. |
| **Base / head** | The two git revisions compared. Usually the branch a pull request targets, and the pull request itself. |
| **Query catalog** | The named queries in `plantwin.yaml` whose plans are compared. |
| **Finding** | One problem the rules detected, with a rule name, a severity, the reason it matters, and a fix. |
| **Estimated** | A label on findings that depend on statistics the twin estimated or could not import. |
| **Accepted finding** | A finding a team has reviewed and chosen to keep, recorded in `.plantwin/accepted.json`. |

## 2. Installation

PlanTwin is not yet published as a release. Build it from a clone of this repository.

**Requirements**: Docker (running), Git, and `make`. Go is not required; the build uses a pinned `golang:1.24` container.

```bash
git clone <this-repository> plantwin && cd plantwin
make image        # builds plantwin-postgres:18: PostgreSQL 18 with the extension installed (not preloaded)
make build        # builds bin/plantwin
export PATH="$PWD/bin:$PATH"
plantwin version
```

Build the twin image in the environment that runs `check` (a developer machine or a CI runner), or pull it from your
registry and tag it as `plantwin-postgres:18`. The image must contain `plantwin.so`; plain `postgres:18` images do not.

## 3. Workflow at a glance

```
 ┌─ production replica ─────────────┐        ┌─ repository / CI ───────────────────────────┐
 │ plantwin export --sign-key …     │ bundle │ plantwin check --base main --head HEAD     │
 │ (read-only role, scheduled)      │ ─────► │ (per pull request)                          │
 └──────────────────────────────────┘        └─────────────────────────────────────────────┘
```

1. Once per repository: `plantwin init`, `plantwin keygen`, and a restricted role on the replica.
2. On a schedule (for example nightly or weekly): `plantwin export`, then publish the bundle and its signature to the
   repository or to a CI artifact store.
3. On each pull request: `plantwin check`. Read the report. Fix the migration, or accept a finding with a reason.

## 4. Set up a repository

```bash
cd your-repo
plantwin init
```

`init` looks for migration tools and writes `plantwin.yaml` with the first one it finds. It also creates a placeholder
query file under `queries/`. Then:

1. **Check the migration tool.** For plain SQL, `migrations.path` points at the directory of `*.sql` files, applied in
   name order. For other tools, see [migration tools](#migration-tools) below.
2. **Write the query catalog.** Replace the placeholder with the queries your application runs most, or the queries that
   matter for the tables you change. Use `$1`, `$2` for parameters. Start with the queries on your hot paths and the
   tables that migrations change most often.
3. **Choose redaction.** The default `hash` is safe. Use `keep` for low-cardinality categorical columns that your queries
   filter on (for example `status`), after reading [privacy](privacy-and-redaction.md).
4. **Generate a signing key** (see below) and commit only the public key.

```bash
plantwin keygen --priv plantwin.key --pub plantwin.pub
echo "plantwin.key" >> .gitignore      # the private key must never be committed
git add plantwin.pub plantwin.yaml queries/
```

Run `plantwin doctor` at any time to see what is missing.

### Migration tools

`init` recognises these and sets the tool accordingly.

| Tool | Detected by | Configuration | Runs on |
|---|---|---|---|
| Plain SQL | `migrations/*.sql` | `migrations: {tool: sql, path: migrations}` | the twin, directly |
| Flyway | `flyway.conf` or `sql/V*__*.sql` | `tool: sql`, `path: sql` | the twin, directly (repeatable `R__` files are not supported) |
| Django | `manage.py` with `*/migrations/*.py` | `tool: command`, `command: python manage.py migrate` | the host |
| Rails | `db/migrate` and `config/database.yml` | `tool: command`, `command: bin/rails db:migrate` | the host |
| Prisma | `prisma/schema.prisma` | `tool: command`, `command: npx prisma migrate deploy` | the host |
| Alembic | `alembic.ini` | `tool: command`, `command: alembic upgrade head` | the host |
| Atlas | `atlas.hcl` | `tool: command`, `command: atlas migrate apply --url "$DATABASE_URL" --dir file://migrations` | the host |
| None | nothing detected | `tool: none` | — |

For `command`, the command runs on the machine that runs `check`, in the revision's worktree, with `DATABASE_URL` set to
the twin. Install the tool's runtime on that machine first.

## 5. Export statistics from a replica

Run export on a **read replica**, never on the primary, using a role that cannot read table rows.

### 5.1 Create the role

As a superuser on the replica (replace the names and the password):

```sql
CREATE ROLE plantwin_export LOGIN PASSWORD '<generated secret>' NOSUPERUSER NOCREATEDB NOCREATEROLE;
GRANT CONNECT ON DATABASE app TO plantwin_export;
GRANT USAGE ON SCHEMA public TO plantwin_export;     -- one line for each schema that holds exported tables
GRANT SELECT ON pg_statistic TO plantwin_export;     -- the only privilege beyond connecting
```

The role can read statistics, including common values, but no table rows. Protect it like any production read credential.
Full explanation: [privacy-and-redaction.md](privacy-and-redaction.md#export-role).

### 5.2 Run the export

```bash
export PLANTWIN_DSN='postgres://plantwin_export:<secret>@replica.internal:5432/app?sslmode=require'
plantwin export --config plantwin.yaml --sign-key ./plantwin.key --out plantwin-bundle.json
```

The command writes `plantwin-bundle.json` and `plantwin-bundle.json.sig`. It prints how many relations and column
statistics it exported, the redaction applied, and any warnings. Examples of warnings:

- extended statistics exist and will not be restored;
- a table has pages but no tuple estimate, so someone should run `ANALYZE` on it.

PlanTwin never runs `ANALYZE` itself.

Use a different environment variable with `--dsn-env` if `PLANTWIN_DSN` is taken. Keep the DSN out of shell history and
out of logs.

### 5.3 Publish the bundle

Commit `plantwin-bundle.json` and `plantwin-bundle.json.sig` to the repository, or store them in a CI artifact or object
store that the check job can read. Refresh the bundle regularly. `doctor` warns when it is more than 14 days old.

## 6. Run a check

```bash
plantwin check --base main --head HEAD
```

| Flag | Default | Meaning |
|---|---|---|
| `--config` | `plantwin.yaml` | Configuration file. The repository root is its directory. |
| `--base` | `main` | Base git ref. Use the pull request's target branch in CI. |
| `--head` | `HEAD` | Head git ref. Use `WORKTREE` to test uncommitted changes. |
| `--format` | `md` | `md`, `json` or `sarif`. |
| `--out` | stdout | Write the report to a file. |
| `--bundle` | `bundle.path` | Use a different bundle for this run. |
| `--parallel` | `4` | EXPLAIN connections per twin. |
| `--keep` | off | Keep the twin containers after the run, for debugging. |
| `--verbose` | off | Show the underlying error for failures. |

What happens, in order:

1. The bundle signature is verified. A mismatch stops the run.
2. Git worktrees are created for base and head.
3. Two twins start in parallel, migrations are applied (or replayed from the schema cache), and the statistics are
   imported.
4. Tables changed by the migration are labelled *estimated*.
5. Every query in the catalog is explained on both twins.
6. Plans are compared, rules are applied, accepted findings are removed, and the report is written.
7. Twins are removed, unless `--keep` is given.

To check uncommitted work locally: `plantwin check --base HEAD --head WORKTREE`.

## 7. Read the report

A Markdown report has four parts.

**Header.** The base and head refs, the result (passing or failing), and counts by severity. A failing result means at least
one finding met `fail_on`.

**Notes.** Estimated relations, and warnings. Read these first. If a relation is estimated, its findings are less certain,
and the report says so on each finding.

**Findings.** For each query, each finding has:

- a severity (`ERROR`, `WARNING`, `INFO`),
- the rule name (see [rules.md](rules.md)),
- the message, with the table involved,
- *Why this matters*, one line,
- *Suggested fix*, one line.

**Side-by-side plans.** A collapsible block with both trees, operator by operator, with rows and cost. Compare the
operators first; estimates matter less than what changed.

Then decide:

| Situation | What to do |
|---|---|
| The change is intended and the plan is acceptable | `plantwin accept <query> <rule> [relation]` and say why in the pull request |
| The change is unintended | Fix the migration, for example by recreating the index in the same migration |
| The finding is labelled *estimated* and you doubt it | Check the table in production (`EXPLAIN` on the real query), and consider re-exporting after `ANALYZE` |
| A `query_error` appears | The query no longer plans on head (often a type change). Fix the query or the migration |

`plantwin explain <query>` prints the two plan trees for one query from the last run, with its findings.

## 8. Command reference

### `plantwin init`

```
plantwin init [-dir .]
```

Detects migration tools in `-dir`, writes `plantwin.yaml` and a placeholder query. Refuses to overwrite an existing
configuration.

### `plantwin keygen`

```
plantwin keygen [-priv plantwin.key] [-pub plantwin.pub]
```

Writes an ed25519 key pair. The private key is created with mode `0600`.

### `plantwin export`

```
plantwin export [-config plantwin.yaml] [-dsn-env PLANTWIN_DSN] [-out plantwin-bundle.json] -sign-key <file>
```

Exports from the database named in the environment variable. `-sign-key` is required; an unsigned export is not
supported.

### `plantwin check`

See [section 6](#6-run-a-check).

### `plantwin explain`

```
plantwin explain <query> [-repo .]
```

Reads `.plantwin/last-run.json`. Prints both plan trees and the findings for one query. Fails with a clear message if
no check has run yet, or the query name is unknown.

### `plantwin accept`

```
plantwin accept <query> <rule> [<relation>] [-repo .]
```

Adds an entry to `.plantwin/accepted.json`. The rule must be one of `lost_index`, `seq_scan_large`, `join_method`,
`sort_hash_work_mem`, `cost_ratio`, `shape_changed`, `query_error`. Omit the relation to accept the rule for any relation
in that query. Commit the file, so the decision is reviewed.

### `plantwin doctor`

```
plantwin doctor [-config plantwin.yaml]
```

Checks, in order: Docker, configuration, twin image (exists, runs PostgreSQL 18), the extension in the image, Git, bundle
presence, signature, age (14 days), stats completeness, extended statistics, and bundle warnings. Each failure says what
to do. Exits `1` if any check fails.

### `plantwin demo`

```
plantwin demo [-keep] [-format md|json|sarif] [-redaction keep|hash|drop-mcv|drop-histogram]
```

Builds a temporary repository with two commits, a synthetic production database (500,000 rows), a signed bundle with an
ephemeral key, and runs a check. Use it to see what the output looks like before you configure your own repository.

### `plantwin version`

Prints the version.

### Global

`--verbose` may be placed anywhere on the command line. It shows the underlying error for failures.

## 9. Configuration reference

A complete example with every key:

```yaml
version: 1

# Twin image. Must contain plantwin.so (build with `make image`).
image: plantwin-postgres:18

migrations:
  tool: sql                 # sql | command | none
  path: migrations          # sql: directory of *.sql, applied in name order
  # command: "python manage.py migrate"   # command: run on the host with DATABASE_URL set to the twin

queries:
  - name: customer_lookup
    sql: "SELECT id, status, total FROM orders WHERE customer_id = $1"
  - name: open_orders
    file: queries/open_orders.sql        # use sql or file, not both

thresholds:                # all optional; defaults shown
  large_rows: 100000       # a lost index on a table this large is an error
  seq_scan_rows: 10000     # a new sequential scan on a table this large is reported
  join_rows: 10000         # a join-method change producing this many rows is reported
  work_mem_bytes: 4194304  # Sort or Hash estimated above this is reported
  cost_ratio: 2.0          # info only
  min_cost: 1000           # the cost rule ignores cheaper plans
  index_selectivity: 0.10  # an index returning at least this share of its table is not "selective"

redaction:
  default: hash            # keep | hash | drop-mcv | drop-histogram
  columns:
    orders.status: keep    # table.column overrides

fail_on: warning           # error | warning | info

bundle:
  path: plantwin-bundle.json
  public_key: plantwin.pub
```

Rules for values:

- `queries[].name` must be unique. Each query needs exactly one of `sql` or `file`.
- `redaction.default` and every `redaction.columns` value must be a known mode.
- `index_selectivity` must be greater than 0 and at most 1.
- `fail_on` controls which findings fail the check. `error` fails only on errors; `warning` (default) fails on errors and
  warnings; `info` fails on anything.

Invalid configuration fails before any container starts, with the field named in the message.

## 10. Continuous integration

PlanTwin's own CI runs on every pull request and on `main`, and the examples workflow tests the published release. The
assets below are the ones PlanTwin itself uses; see [docs/ci-triage.md](ci-triage.md) for the failures that were fixed.

### 10.1 Requirements for a CI job

- Docker available to the job, and the twin image available to that Docker daemon.
- The signed bundle and public key in the repository, or fetched from an artifact.
- No export DSN. Export runs elsewhere, on a schedule.
- Checks on pull requests from forks should not have access to secrets (see [SECURITY.md](../SECURITY.md)).

### 10.2 GitHub Actions

The Action is published at this repository's tags and listed on the [GitHub Marketplace](https://github.com/marketplace/actions/plantwin-check). Use `v0.1.2` or later (`v0.1.0` cannot install the CLI; see
[STATUS](STATUS.md)). It downloads the release binary, verifies its checksum, and runs `plantwin check`. It does **not**
provide the twin image, so the job must make `plantwin-postgres:18` available first. The published image is public:

```yaml
name: plantwin
on:
  pull_request:
permissions:
  contents: read
jobs:
  check:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0                   # needed to resolve the base branch
          persist-credentials: false
      - name: Provide the twin image (published, signed)
        run: |
          docker pull ghcr.io/raunaksahahere/plantwin-postgres:18
          docker tag ghcr.io/raunaksahahere/plantwin-postgres:18 plantwin-postgres:18
      - name: Run PlanTwin
        uses: raunaksahahere/planTwin@v0.1.2
        with:
          base: origin/${{ github.base_ref }}
          head: HEAD
          config: plantwin.yaml
          format: md
          report: plantwin-report.md
      - name: Show the report
        if: always()
        run: cat plantwin-report.md >> "$GITHUB_STEP_SUMMARY"
```

The Action's step fails when the check fails, so the job is red on a regression. The inputs are `version` (the release
binary, default `v0.1.2`), `base`, `head`, `config`, `format` and `report`.

What is verified: the examples workflow runs this Action at `v0.1.2`, with the same input names, against a scratch
repository it builds, with `base` set to the previous commit (`HEAD~1`). It asserts the planted regression
([STATUS](STATUS.md)). What is not verified: the `origin/<branch>` form above, and a repository other than this one.

### 10.3 GitLab CI

**Not run, and it needs a change before use.** The template downloads the CLI from a GitLab generic package registry, and
this project publishes releases only to GitHub. Mirror the release files to a registry you control, and set
`PLANTWIN_PROJECT_ID` to that location, before the template can run.

`action/gitlab-ci.yml` is a template for merge-request pipelines. It needs:

- `PLANTWIN_PROJECT_ID` (where the release packages live) and `PLANTWIN_VERSION`, once a release exists;
- `PLANTWIN_IMAGE`, a registry image that will be tagged as `plantwin-postgres:18` inside the job.

```yaml
include:
  - local: action/gitlab-ci.yml
variables:
  PLANTWIN_IMAGE: registry.example.com/platform/plantwin-postgres:18
```

## 11. Accepting findings

```bash
plantwin accept customer_lookup lost_index orders
git add .plantwin/accepted.json && git commit -m "Accept lost_index on orders: index replaced by partial index"
```

Accepted findings are not counted, and the report notes how many were accepted. Accepting is a review decision, so keep
the reason in the commit or the pull request. Remove an entry from `accepted.json` to make the finding count again.

## 12. Exit codes

| Code | Meaning |
|---|---|
| `0` | Check passed (no finding met `fail_on`), or the command succeeded |
| `1` | Check failed: at least one finding met `fail_on`; or `doctor` found a failed check |
| `2` | Usage or configuration error, or a missing or invalid input (bundle, key, query name) |
| `3` | Docker or Git is unavailable |

## 13. Troubleshooting

Run `plantwin doctor` first. For the underlying error of any failure, add `--verbose`.

| Message (abridged) | Cause | Fix |
|---|---|---|
| `Docker is not running or not installed.` | Docker daemon is down or not on `PATH` | Start Docker; check `docker info` |
| `Could not start the PostgreSQL container from image …` | Image not built or not present locally | `make image`, or set `image:` to an image you have |
| `The plantwin extension is not available in the twin image.` | Image is plain `postgres:18` | Build the image from this repository's Dockerfile |
| `The stats bundle is not signed.` | No `.sig` file beside the bundle | Re-export with `--sign-key` |
| `The stats bundle signature does not verify.` | Bundle changed after export, or wrong public key | Re-export; check `bundle.public_key` |
| `The export role cannot read the planner statistics catalog (pg_statistic).` | Missing grant | `GRANT SELECT ON pg_statistic TO <role>;` on the replica |
| `Cannot connect to the export database.` | Network, DSN, or the role lacks `LOGIN` | Check the DSN and the route to the replica |
| `Migration <file> failed on the twin.` | The migration does not run on an empty database, or needs an extension the twin lacks | Run it against an empty database to see the full error |
| `No .sql migration files were found in …` | `migrations.path` is wrong | Correct the path |
| `No query named "…" in the last check.` | Name mismatch, or the last check predates the query | Use a name from `plantwin.yaml`, then run `check` again |
| `… is not a valid git ref.` | Ref is empty or starts with `-` | Use a branch, tag or commit |
| `The PostgreSQL container did not become ready within 90 seconds.` | Container failed to start | `docker logs <container>` |
| `plantwin.yaml was not found …` | Not in the repository root | Run `plantwin init`, or pass `--config` |
| *estimated* on a finding you did not expect | The migration changed that table | Expected; check the table's definition in production |
| `query_error` finding | The query no longer plans on head | Usually a type change; update the query or migration |
| Report says *statistics object could not be restored* | Object changed in the migration | Expected for changed objects; check the note |

## 14. Operating PlanTwin

- **Refresh bundles.** Export on a schedule that matches how often statistics move in production. `doctor` warns after 14
  days.
- **Keep the query catalog current.** A check only sees the queries you list. Review the catalog when hot paths change.
- **Watch for drift.** A bundle that has not been refreshed, or statistics that have not been analysed in production,
  make findings less reliable. The report labels estimated findings; `doctor` reports stats completeness.
- **Storage.** Twins are disposable. The schema cache in `.plantwin/cache/` is small; delete it at any time.
- **Redaction review.** When a new sensitive column appears, add it to `redaction.columns` or confirm the default covers it.
  Re-run `plantwin doctor` after changing redaction.
- **Upgrades.** A bundle records the PostgreSQL version it was exported from. The twin must be PostgreSQL 18 in v0.1.
