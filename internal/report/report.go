// Package report turns Grafana incidents into flat, exportable rows.
package report

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	incident "github.com/grafana/incident-go"
)

// Row is one incident flattened for export. Roles maps a role name to the
// formatted list of users holding that role in this incident.
type Row struct {
	Title    string
	Status   string
	Severity string
	Declared string // RFC3339, from CreatedTime
	Resolved string // RFC3339, from ClosedTime (empty while open)
	Labels   string // "key=label" pairs joined by "; "
	Roles    map[string]string
	Debrief  string // key updates as "timestamp: text" joined by "; "
}

// Report is the set of rows plus the role columns to emit. Roles become
// individual CSV columns, one per role type.
type Report struct {
	RoleColumns []string // fixed role columns, in order
	Rows        []Row
}

// baseHeader is the fixed leading columns; role columns are appended after.
var baseHeader = []string{
	"title", "status", "severity", "declared", "resolved", "labels",
}

// roleColumns are the role types we emit a column for, in order, always present
// even when no one holds the role in a given incident. The first three are
// Grafana's predefined incident roles; "observer" is what the IRM page shows as
// chat participants. Any other role found in the data is dropped.
var roleColumns = []string{"commander", "investigator", "communicator", "observer"}

// Build flattens incidents into a Report. emails maps a user ID to an email
// address (users missing from it are rendered by name only); debriefs maps an
// incident ID to its key updates.
func Build(incidents []incident.Incident, emails map[string]string, debriefs map[string][]incident.KeyUpdate) Report {
	rows := make([]Row, 0, len(incidents))
	for _, inc := range incidents {
		rows = append(rows, Row{
			Title:    inc.Title,
			Status:   inc.Status,
			Severity: inc.Severity,
			Declared: inc.CreatedTime,
			Resolved: inc.ClosedTime,
			Labels:   formatLabels(inc.Labels),
			Roles:    groupRoles(inc.IncidentMembership.Assignments, emails),
			Debrief:  formatDebrief(debriefs[inc.IncidentID]),
		})
	}
	return Report{RoleColumns: roleColumns, Rows: rows}
}

// formatDebrief renders key updates as "timestamp: text" entries joined by
// "; ", in the order given (chronological). Empty updates are skipped.
func formatDebrief(updates []incident.KeyUpdate) string {
	parts := make([]string, 0, len(updates))
	for _, u := range updates {
		text := u.Content
		if text == "" && u.Title != nil {
			text = *u.Title
		}
		text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
		if text == "" {
			continue
		}
		parts = append(parts, shortTime(u.CreatedTime)+": "+text)
	}
	return strings.Join(parts, "; ")
}

// shortTime trims an RFC3339 timestamp to minute precision in UTC, falling back
// to the raw value if it can't be parsed.
func shortTime(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return t.UTC().Format("2006-01-02 15:04Z")
}

// groupRoles maps each role name to its "; "-joined formatted users.
func groupRoles(assignments []incident.Assignment, emails map[string]string) map[string]string {
	byRole := map[string][]string{}
	for _, a := range assignments {
		byRole[a.Role.Name] = append(byRole[a.Role.Name], formatUser(a.User, emails))
	}
	out := make(map[string]string, len(byRole))
	for role, users := range byRole {
		out[role] = strings.Join(users, "; ")
	}
	return out
}

// formatUser renders a user as "name <email>", or just "name" when the email is
// unknown.
func formatUser(u incident.UserPreview, emails map[string]string) string {
	if email := emails[u.UserID]; email != "" {
		return fmt.Sprintf("%s <%s>", u.Name, email)
	}
	return u.Name
}

// formatLabels renders labels as "key=label" pairs (or just the label when no
// key), joined by "; " and sorted for stable output.
func formatLabels(labels []incident.IncidentLabel) string {
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		if l.Key != "" {
			parts = append(parts, l.Key+"="+l.Label)
		} else {
			parts = append(parts, l.Label)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

// header is the full column list: the fixed columns, one column per role type,
// then debrief.
func (r Report) header() []string {
	h := append(append([]string{}, baseHeader...), r.RoleColumns...)
	return append(h, "debrief (key updates)")
}

// record flattens one row into a slice aligned with header().
func (r Report) record(row Row) []string {
	rec := []string{row.Title, row.Status, row.Severity, row.Declared, row.Resolved, row.Labels}
	for _, role := range r.RoleColumns {
		rec = append(rec, row.Roles[role])
	}
	return append(rec, row.Debrief)
}

// WriteCSV writes the report as CSV: the fixed columns followed by one column
// per role type, then debrief.
func WriteCSV(w io.Writer, r Report) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(r.header()); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, row := range r.Rows {
		if err := cw.Write(r.record(row)); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteMarkdown writes the report as a GitHub-flavored Markdown table with the
// same columns as the CSV form.
func WriteMarkdown(w io.Writer, r Report) error {
	bw := bufio.NewWriter(w)
	header := r.header()
	writeMarkdownRow(bw, header)
	writeMarkdownRow(bw, make([]string, len(header))) // empty cells -> all "---"
	for _, row := range r.Rows {
		writeMarkdownRow(bw, r.record(row))
	}
	return bw.Flush()
}

// writeMarkdownRow writes one table row. A row of empty cells renders as the
// header/body separator (each cell becomes "---").
func writeMarkdownRow(w *bufio.Writer, cells []string) {
	allEmpty := true
	for _, c := range cells {
		if c != "" {
			allEmpty = false
			break
		}
	}
	w.WriteString("|")
	for _, c := range cells {
		if allEmpty {
			w.WriteString(" --- |")
		} else {
			w.WriteString(" " + escapeMarkdownCell(c) + " |")
		}
	}
	w.WriteString("\n")
}

// escapeMarkdownCell makes a value safe inside a Markdown table cell: pipes are
// escaped and newlines become <br> so a cell never breaks the table.
func escapeMarkdownCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "<br>")
	return s
}
