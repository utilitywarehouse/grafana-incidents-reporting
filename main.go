// Command grafana-incidents-reporting pulls incidents from the Grafana
// Incidents API for a chosen time window and writes a CSV report.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/utilitywarehouse/grafana-incidents-reporting/internal/grafana"
	"github.com/utilitywarehouse/grafana-incidents-reporting/internal/report"
	"github.com/utilitywarehouse/grafana-incidents-reporting/internal/timerange"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		apiURL   = flag.String("api-url", os.Getenv("GRAFANA_INCIDENT_API_URL"), "Grafana Incidents API base URL (env GRAFANA_INCIDENT_API_URL)")
		days     = flag.Int("days", 0, "report on incidents declared in the past N days")
		month    = flag.String("month", "", "report on a calendar month (YYYY-MM)")
		day      = flag.String("day", "", "report on a single day (YYYY-MM-DD)")
		from     = flag.String("from", "", "explicit window start (RFC3339 or YYYY-MM-DD)")
		to       = flag.String("to", "", "explicit window end (RFC3339 or YYYY-MM-DD)")
		statuses = flag.String("statuses", "", "comma-separated statuses to include (default: all)")
		drills   = flag.Bool("include-drills", false, "include drill incidents")
		output   = flag.String("output", "", "output file path (default: stdout)")
	)
	flag.Usage = usage
	flag.Parse()

	token := os.Getenv("SERVICE_ACCOUNT_TOKEN")
	if *apiURL == "" {
		return fmt.Errorf("missing API URL: set --api-url or GRAFANA_INCIDENT_API_URL")
	}
	if token == "" {
		return fmt.Errorf("missing token: set SERVICE_ACCOUNT_TOKEN")
	}

	sel := timerange.Selector{Days: *days, Month: *month, Day: *day, From: *from, To: *to}
	window, err := sel.Resolve(time.Now())
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := grafana.New(*apiURL, token)

	incidents, err := client.ListIncidents(ctx, grafana.QueryParams{
		Range:           window,
		IncludeDrills:   *drills,
		IncludeStatuses: splitCSV(*statuses),
	})
	if err != nil {
		return err
	}

	emails, err := client.ResolveAssigneeEmails(ctx, incidents)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning:", err)
	}

	rep := report.Build(incidents, emails)

	out := os.Stdout
	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		out = f
	}
	if err := report.WriteCSV(out, rep); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "wrote %d incident(s) for %s .. %s\n",
		len(rep.Rows), window.From.Format(time.RFC3339), window.To.Format(time.RFC3339))
	return nil
}

// splitCSV splits a comma-separated list, trimming spaces and dropping empties.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func usage() {
	fmt.Fprintf(os.Stderr, `grafana-incidents-reporting — export Grafana incidents to CSV.

Usage:
  grafana-incidents-reporting [flags]

Time window (pick one; defaults to the past 7 days):
  --days N            past N days
  --month YYYY-MM     a calendar month
  --day YYYY-MM-DD    a single day
  --from / --to       explicit range (RFC3339 or YYYY-MM-DD)

Environment:
  GRAFANA_INCIDENT_API_URL   API base URL (or --api-url)
  SERVICE_ACCOUNT_TOKEN      Grafana service account token (required)

Flags:
`)
	flag.PrintDefaults()
}
