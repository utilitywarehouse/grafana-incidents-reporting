# grafana-incidents-reporting

A small Go CLI that pulls incidents from the [Grafana Incidents API][api] (via
[`grafana/incident-go`][sdk]) for a chosen time window and writes a report.

Each row has these fixed columns:

| Column | Source |
| --- | --- |
| `title` | incident title |
| `status` | incident status |
| `severity` | incident severity |
| `declared` | `createdTime` (RFC3339) |
| `resolved` | `closedTime` (RFC3339, empty while open) |
| `labels` | `key=label` pairs, e.g. teams |
| `commander` | users holding the commander role |
| `investigator` | users holding the investigator role |
| `communicator` | users holding the communicator role |
| `observer` | chat participants |
| `debrief (key updates)` | the incident's key updates, chronologically |

The four role columns are always present (empty when no one holds the role).
`commander`, `investigator`, and `communicator` are Grafana's predefined incident
roles; `observer` is what the IRM page shows as chat participants. Any other role
type in the data is dropped. Each user is rendered as `name <email>`, with
multiple users in a role separated by `; `. Emails are resolved from each user's
record; if a user can't be resolved, just the name is shown.

`debrief` is built from the incident's key updates (the status updates posted
during the incident), rendered as `YYYY-MM-DD HH:MMZ: text` entries joined by
`; ` in chronological order.

## Configuration

Two settings come from the environment:

| Variable | Meaning |
| --- | --- |
| `GRAFANA_INCIDENT_API_URL` | API base, e.g. `https://<stack>.grafana.net/api/plugins/grafana-irm-app/resources/api/v1` (also `--api-url`) |
| `SERVICE_ACCOUNT_TOKEN` | Grafana service account token ([docs][auth]) |

## Usage

The tool runs on demand and reports on incidents **declared** within a window.
Pick exactly one way to specify the window (defaults to the past 7 days):

```sh
# Past N days
grafana-incidents-reporting --days 14

# A whole calendar month (explicit, or the current / previous one)
grafana-incidents-reporting --month 2026-05
grafana-incidents-reporting --month current
grafana-incidents-reporting --month previous

# A single day
grafana-incidents-reporting --day 2026-06-10

# An explicit range (RFC3339 or YYYY-MM-DD)
grafana-incidents-reporting --from 2026-06-01 --to 2026-06-08
```

Other flags:

```
--statuses active,resolved   only include these statuses (default: all)
--include-drills             include drill incidents (excluded by default)
--format csv|md              output format (default csv)
--output report.csv          write a local copy to a file
```

## Output format

`--format csv` (default) writes the CSV described above. `--format md` writes a
per-incident Markdown **list** rather than a table: one block per incident with
each column as a `**label:** value` line, blocks separated by a horizontal rule
(`---`). Wide tables render poorly on narrow viewers (e.g. Slab); a list keeps
every field on its own line and reads cleanly. For example:

```markdown
**title:** Checkout is down
**status:** resolved
**severity:** critical
**declared:** 2026-06-10T09:00:00Z
**resolved:** 2026-06-10T10:30:00Z
**labels:** service=checkout; team=payments
**commander:** alice <alice@uw.co.uk>
**investigator:** bob <bob@uw.co.uk>; carol
**communicator:**
**observer:**
**debrief (key updates):** 2026-06-10 09:12Z: Found root cause

---

**title:** API latency spike
...
```

## Pushing to a git repo

The report can be committed straight to a git repository over SSH using a
**deploy key**. The filename is **deterministic from the window** so
re-running the same report overwrites the previous file rather than piling up
copies:

| Window | Filename |
| --- | --- |
| `--month 2026-05` / `--month current` | `grafana-irm-incidents-2026-05.md` |
| `--day 2026-06-10` | `grafana-irm-incidents-2026-06-10.md` |
| `--from … --to …` / `--days N` | `grafana-irm-incidents-2026-06-01_2026-06-08.md` |

```sh
export GITHUB_DEPLOY_KEY=/etc/incidents/ssh-key   # read/write deploy key
grafana-incidents-reporting --month previous --format md \
  --git-repo my-org/incident-reports --git-path reports
```

This creates or overwrites `reports/grafana-irm-incidents-2026-05.md` on the repo's default
branch in one commit. To keep each run cheap, the clone is shallow (one commit),
partial (blobs fetched on demand), and sparse (only `--git-path` is checked
out). If the file already holds identical content, nothing is committed. Flags:

```
--git-repo owner/repo     target repository (owner/repo, or a full SSH URL)
--git-path DIR            directory under the repo (default: repo root)
--git-branch BRANCH       branch to push to (default: the repo's default branch)
--ssh-key PATH            deploy key for the push (env GITHUB_DEPLOY_KEY)
--git-author-name NAME    commit author name (env GIT_AUTHOR_NAME)
--git-author-email MAIL   commit author email (env GIT_AUTHOR_EMAIL)
```

`--git-repo` accepts the `owner/repo` shorthand (expanded to
`git@github.com:owner/repo.git`) or any SSH URL for self-hosted/other remotes.
The deploy key must have **write access** (add it to the repo with "Allow write
access" enabled). Commits default to a generic `grafana-incidents-reporting`
author; set `--git-author-name`/`--git-author-email` (or the matching env vars)
to attribute them to your own team.

When `--git-repo` is set, the report is **not** echoed to stdout; pass
`--output` as well if you also want a local copy.

Example:

```sh
export GRAFANA_INCIDENT_API_URL="https://my-stack.grafana.net/api/plugins/grafana-irm-app/resources/api/v1"
export SERVICE_ACCOUNT_TOKEN="glsa_..."
grafana-incidents-reporting --month 2026-05 --output may-incidents.csv
```
