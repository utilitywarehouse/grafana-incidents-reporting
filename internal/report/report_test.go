package report

import (
	"bytes"
	"strings"
	"testing"

	incident "github.com/grafana/incident-go"
)

func TestBuildAndWriteCSV(t *testing.T) {
	incidents := []incident.Incident{{
		IncidentID:  "inc-1",
		Title:       "Checkout is down",
		Status:      "resolved",
		Severity:    "critical",
		CreatedTime: "2026-06-10T09:00:00Z",
		ClosedTime:  "2026-06-10T10:30:00Z",
		Labels: []incident.IncidentLabel{
			{Key: "team", Label: "payments"},
			{Key: "service", Label: "checkout"},
		},
		IncidentMembership: incident.IncidentMembership{
			Assignments: []incident.Assignment{
				{User: incident.UserPreview{Name: "Alice"}, Role: incident.Role{Name: "Commander"}},
				{User: incident.UserPreview{Name: "Bob"}, Role: incident.Role{Name: "Investigator"}},
				{User: incident.UserPreview{Name: "Carol"}, Role: incident.Role{Name: "Investigator"}},
			},
		},
	}}

	rows := Build(incidents)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}

	r := rows[0]
	if r.Labels != "service=checkout; team=payments" {
		t.Errorf("Labels = %q", r.Labels)
	}
	if r.Roles != "Commander: Alice; Investigator: Bob, Carol" {
		t.Errorf("Roles = %q", r.Roles)
	}

	var buf bytes.Buffer
	if err := WriteCSV(&buf, rows); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "title,status,severity,declared,resolved,labels,roles\n") {
		t.Errorf("missing/incorrect header: %q", out)
	}
	if !strings.Contains(out, "Checkout is down,resolved,critical,") {
		t.Errorf("row not rendered: %q", out)
	}
}
