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

// Row is one incident flattened for export.
type Row struct {
	Title    string
	Status   string
	Severity string
	Declared string // RFC3339, from CreatedTime
	Resolved string // RFC3339, from ClosedTime (empty while open)
	Labels   string // "key=label" pairs joined by "; "
	Roles    string // "Role: User1, User2" groups joined by "; "
}

// header is the CSV column order.
var header = []string{
	"title", "status", "severity", "declared", "resolved",
	"labels", "roles",
}

// Build flattens incidents into rows.
func Build(incidents []incident.Incident) []Row {
	rows := make([]Row, 0, len(incidents))
	for _, inc := range incidents {
		rows = append(rows, Row{
			Title:    inc.Title,
			Status:   inc.Status,
			Severity: inc.Severity,
			Declared: inc.CreatedTime,
			Resolved: inc.ClosedTime,
			Labels:   formatLabels(inc.Labels),
			Roles:    formatRoles(inc.IncidentMembership.Assignments),
		})
	}
	return rows
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

// formatRoles groups assignments by role name.
func formatRoles(assignments []incident.Assignment) string {
	byRole := map[string][]string{}
	order := []string{}
	for _, a := range assignments {
		role := a.Role.Name
		if _, seen := byRole[role]; !seen {
			order = append(order, role)
		}
		byRole[role] = append(byRole[role], a.User.Name)
	}
	sort.Strings(order)
	parts := make([]string, 0, len(order))
	for _, role := range order {
		parts = append(parts, fmt.Sprintf("%s: %s", role, strings.Join(byRole[role], ", ")))
	}
	return strings.Join(parts, "; ")
}

// WriteCSV writes rows (with a header) as CSV to w.
func WriteCSV(w io.Writer, rows []Row) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, r := range rows {
		rec := []string{r.Title, r.Status, r.Severity, r.Declared, r.Resolved, r.Labels, r.Roles}
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}
