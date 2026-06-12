# grafana-incidents-reporting

A small Go CLI that pulls incidents from the [Grafana Incidents API][api] (via
[`grafana/incident-go`][sdk]) for a chosen time window and writes a CSV report.

Each row contains:

| Column | Source |
| --- | --- |
| `title` | incident title |
| `status` | incident status |
| `severity` | incident severity |
| `declared` | `createdTime` (RFC3339) |
| `resolved` | `closedTime` (RFC3339, empty while open) |
| `labels` | `key=label` pairs, e.g. teams |
| `roles` | assignments grouped by role, e.g. `Commander: Alice; Investigator: Bob, Carol` |

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

# A whole calendar month
grafana-incidents-reporting --month 2026-05

# A single day
grafana-incidents-reporting --day 2026-06-10

# An explicit range (RFC3339 or YYYY-MM-DD)
grafana-incidents-reporting --from 2026-06-01 --to 2026-06-08
```

Other flags:

```
--statuses active,resolved   only include these statuses (default: all)
--include-drills             include drill incidents (excluded by default)
--output report.csv          write to a file instead of stdout
```

Example:

```sh
export GRAFANA_INCIDENT_API_URL="https://my-stack.grafana.net/api/plugins/grafana-irm-app/resources/api/v1"
export SERVICE_ACCOUNT_TOKEN="glsa_..."
grafana-incidents-reporting --month 2026-05 --output may-incidents.csv
```
