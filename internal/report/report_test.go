package report

import (
	"bytes"
	"reflect"
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
				{User: incident.UserPreview{UserID: "u1", Name: "alice"}, Role: incident.Role{Name: "commander"}},
				{User: incident.UserPreview{UserID: "u2", Name: "bob"}, Role: incident.Role{Name: "investigator"}},
				{User: incident.UserPreview{UserID: "u3", Name: "carol"}, Role: incident.Role{Name: "investigator"}},
				{User: incident.UserPreview{UserID: "u4", Name: "dave"}, Role: incident.Role{Name: "scribe"}},
			},
		},
	}}

	// u1/u2 have emails; u3 does not and is rendered by name only.
	emails := map[string]string{"u1": "alice@uw.co.uk", "u2": "bob@uw.co.uk"}

	debriefs := map[string][]incident.KeyUpdate{
		"inc-1": {
			{CreatedTime: "2026-06-10T09:12:00Z", Content: "Found root cause"},
			{CreatedTime: "2026-06-10T10:30:45Z", Content: "Fix deployed,\nmonitoring"},
		},
	}

	rep := Build(incidents, emails, debriefs)
	if len(rep.Rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rep.Rows))
	}
	if got, want := rep.RoleColumns, []string{"commander", "investigator", "communicator", "observer"}; !reflect.DeepEqual(got, want) {
		t.Errorf("RoleColumns = %v, want %v", got, want)
	}

	r := rep.Rows[0]
	if r.Labels != "service=checkout; team=payments" {
		t.Errorf("Labels = %q", r.Labels)
	}
	if r.Roles["commander"] != "alice <alice@uw.co.uk>" {
		t.Errorf("commander = %q", r.Roles["commander"])
	}
	if r.Roles["investigator"] != "bob <bob@uw.co.uk>; carol" {
		t.Errorf("investigator = %q", r.Roles["investigator"])
	}
	// Newlines in update text are collapsed; entries joined chronologically.
	if r.Debrief != "2026-06-10 09:12Z: Found root cause; 2026-06-10 10:30Z: Fix deployed, monitoring" {
		t.Errorf("Debrief = %q", r.Debrief)
	}

	var buf bytes.Buffer
	if err := WriteCSV(&buf, rep); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "title,status,severity,declared,resolved,labels,commander,investigator,communicator,observer,debrief (key updates)\n") {
		t.Errorf("missing/incorrect header: %q", out)
	}
	if !strings.Contains(out, "Checkout is down,resolved,critical,") {
		t.Errorf("row not rendered: %q", out)
	}
	// "scribe" is not a known role column, so it must not appear in the output.
	if strings.Contains(out, "scribe") || strings.Contains(out, "dave") {
		t.Errorf("unexpected role leaked into output: %q", out)
	}
}

func TestWriteMarkdown(t *testing.T) {
	rep := Report{
		RoleColumns: []string{"commander", "communicator"},
		Rows: []Row{
			{
				Title:   "Checkout is down",
				Status:  "resolved",
				Roles:   map[string]string{"commander": "alice"},
				Debrief: "line one\nline two",
			},
			{
				Title:  "API latency spike",
				Status: "active",
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, rep); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	out := buf.String()

	// Fields are bold-labelled lines, not table rows.
	if !strings.Contains(out, "**title:** Checkout is down  \n") {
		t.Errorf("title field missing/incorrect: %q", out)
	}
	// Empty field keeps the label with no trailing value.
	if !strings.Contains(out, "**communicator:**  \n") {
		t.Errorf("empty field not rendered as bare label: %q", out)
	}
	// Newlines within a value collapse so the field stays on one line.
	if !strings.Contains(out, "**debrief (key updates):** line one line two  \n") {
		t.Errorf("multiline value not collapsed: %q", out)
	}
	// A single horizontal rule separates the two incidents.
	if n := strings.Count(out, "\n---\n"); n != 1 {
		t.Errorf("want 1 separator rule, got %d: %q", n, out)
	}
}
