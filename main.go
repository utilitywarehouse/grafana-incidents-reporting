// Command grafana-incidents-reporting pulls incidents from the Grafana
// Incidents API for a chosen time window and writes a CSV report.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/utilitywarehouse/grafana-incidents-reporting/internal/git"
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
		month    = flag.String("month", "", `report on a calendar month: YYYY-MM, "current", or "previous"`)
		day      = flag.String("day", "", "report on a single day (YYYY-MM-DD)")
		from     = flag.String("from", "", "explicit window start (RFC3339 or YYYY-MM-DD)")
		to       = flag.String("to", "", "explicit window end (RFC3339 or YYYY-MM-DD)")
		statuses = flag.String("statuses", "", "comma-separated statuses to include (default: all)")
		drills   = flag.Bool("include-drills", false, "include drill incidents")
		output   = flag.String("output", "", "output file path (default: stdout)")
		format   = flag.String("format", "csv", "output format: csv or md")

		gitRepo   = flag.String("git-repo", "", "push the report to this repo (owner/repo or SSH URL)")
		gitPath   = flag.String("git-path", "", "directory under the repo to write into (default: repo root)")
		gitBranch = flag.String("git-branch", "", "branch to push to (default: the repo's default branch)")
		sshKey    = flag.String("ssh-key", os.Getenv("GITHUB_DEPLOY_KEY"), "path to the deploy key for --git-repo (env GITHUB_DEPLOY_KEY)")

		authorName  = flag.String("git-author-name", os.Getenv("GIT_AUTHOR_NAME"), "commit author name for --git-repo (env GIT_AUTHOR_NAME)")
		authorEmail = flag.String("git-author-email", os.Getenv("GIT_AUTHOR_EMAIL"), "commit author email for --git-repo (env GIT_AUTHOR_EMAIL)")
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
	ext, err := formatExt(*format)
	if err != nil {
		return err
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

	debriefs, err := client.ResolveDebriefs(ctx, incidents)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning:", err)
	}

	rep := report.Build(incidents, emails, debriefs)

	var buf bytes.Buffer
	if err := render(&buf, rep, *format); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "wrote %d incident(s) for %s .. %s\n",
		len(rep.Rows), window.From.Format(time.RFC3339), window.To.Format(time.RFC3339))

	// Write a local copy when asked, or to stdout when there's no other
	// destination. A git push replaces the default stdout dump.
	if *output != "" {
		if err := os.WriteFile(*output, buf.Bytes(), 0o644); err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
	} else if *gitRepo == "" {
		os.Stdout.Write(buf.Bytes())
	}

	if *gitRepo != "" {
		opts := git.Options{
			RepoURL:     *gitRepo,
			Branch:      *gitBranch,
			SSHKey:      *sshKey,
			AuthorName:  *authorName,
			AuthorEmail: *authorEmail,
		}
		if err := pushToGit(ctx, opts, *gitPath, window, ext, buf.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

// render writes the report to w in the given format ("csv" or "md").
func render(w io.Writer, rep report.Report, format string) error {
	switch strings.ToLower(format) {
	case "csv":
		return report.WriteCSV(w, rep)
	case "md", "markdown":
		return report.WriteMarkdown(w, rep)
	default:
		return fmt.Errorf("unknown --format %q (want csv or md)", format)
	}
}

// formatExt returns the file extension for a format, validating it.
func formatExt(format string) (string, error) {
	switch strings.ToLower(format) {
	case "csv":
		return "csv", nil
	case "md", "markdown":
		return "md", nil
	default:
		return "", fmt.Errorf("unknown --format %q (want csv or md)", format)
	}
}

// pushToGit commits content to "<dir>/grafana-irm-incidents-<window>.<ext>" in
// the target repo, creating or overwriting the file in a single commit.
func pushToGit(ctx context.Context, opts git.Options, dir string, window timerange.Range, ext string, content []byte) error {
	name := fmt.Sprintf("grafana-irm-incidents-%s.%s", window.Slug(), ext)
	repoPath := path.Join(dir, name)
	message := fmt.Sprintf("Generated incidents report for %s", window.Slug())
	sha, committed, err := git.Push(ctx, opts, repoPath, content, message)
	if err != nil {
		return err
	}
	if !committed {
		fmt.Fprintf(os.Stderr, "%s already up to date in %s\n", repoPath, opts.RepoURL)
		return nil
	}
	fmt.Fprintf(os.Stderr, "pushed %s to %s (%s)\n", repoPath, opts.RepoURL, sha)
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
  --month YYYY-MM     a calendar month (or "current" / "previous")
  --day YYYY-MM-DD    a single day
  --from / --to       explicit range (RFC3339 or YYYY-MM-DD)

Output:
  --format csv|md     output format (default csv)
  --output PATH       write a local copy to PATH

Push to a git repo over SSH (commits grafana-irm-incidents-<window>.<ext>):
  --git-repo owner/repo   target repository (owner/repo or an SSH URL)
  --git-path DIR          directory under the repo (default: root)
  --git-branch BRANCH     branch (default: the repo's default)
  --ssh-key PATH          deploy key for the push (env GITHUB_DEPLOY_KEY)
  --git-author-name NAME   commit author name (env GIT_AUTHOR_NAME)
  --git-author-email MAIL  commit author email (env GIT_AUTHOR_EMAIL)

Environment:
  GRAFANA_INCIDENT_API_URL   API base URL (or --api-url)
  SERVICE_ACCOUNT_TOKEN      Grafana service account token (required)
  GITHUB_DEPLOY_KEY          path to the SSH deploy key for --git-repo
  GIT_AUTHOR_NAME            commit author name for --git-repo pushes
  GIT_AUTHOR_EMAIL           commit author email for --git-repo pushes

Flags:
`)
	flag.PrintDefaults()
}
