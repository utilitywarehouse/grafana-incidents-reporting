// Package report turns Grafana incidents into flat, exportable rows.
package report

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"

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
// address; users missing from it are rendered by name only.
func Build(incidents []incident.Incident, emails map[string]string) Report {
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
		})
	}
	return Report{RoleColumns: roleColumns, Rows: rows}
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

// WriteCSV writes the report as CSV: the fixed columns followed by one column
// per role type.
func WriteCSV(w io.Writer, r Report) error {
	cw := csv.NewWriter(w)

	header := append(append([]string{}, baseHeader...), r.RoleColumns...)
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, row := range r.Rows {
		rec := []string{row.Title, row.Status, row.Severity, row.Declared, row.Resolved, row.Labels}
		for _, role := range r.RoleColumns {
			rec = append(rec, row.Roles[role])
		}
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}
