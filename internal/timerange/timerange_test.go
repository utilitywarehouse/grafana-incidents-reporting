package timerange

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}

func TestRangeSlug(t *testing.T) {
	time.Local = time.UTC
	tests := []struct {
		name string
		sel  Selector
		want string
	}{
		{name: "month", sel: Selector{Month: "2026-05"}, want: "2026-05"},
		{name: "current month", sel: Selector{Month: "current"}, want: "2026-06"},
		{name: "day", sel: Selector{Day: "2026-06-10"}, want: "2026-06-10"},
		{name: "explicit range", sel: Selector{From: "2026-06-01", To: "2026-06-08"}, want: "2026-06-01_2026-06-08"},
		{name: "past days", sel: Selector{Days: 7}, want: "2026-06-04_2026-06-11"},
	}
	now := mustParse(t, "2026-06-11T12:00:00Z")
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := tc.sel.Resolve(now)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got := r.Slug(); got != tc.want {
				t.Errorf("Slug() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	now := mustParse(t, "2026-06-11T12:00:00Z")

	tests := []struct {
		name     string
		sel      Selector
		wantFrom string
		wantTo   string
		wantErr  bool
	}{
		{name: "default 7 days", sel: Selector{}, wantFrom: "2026-06-04T12:00:00Z", wantTo: "2026-06-11T12:00:00Z"},
		{name: "past 3 days", sel: Selector{Days: 3}, wantFrom: "2026-06-08T12:00:00Z", wantTo: "2026-06-11T12:00:00Z"},
		{name: "month", sel: Selector{Month: "2026-05"}, wantFrom: "2026-05-01T00:00:00Z", wantTo: "2026-06-01T00:00:00Z"},
		{name: "current month", sel: Selector{Month: "current"}, wantFrom: "2026-06-01T00:00:00Z", wantTo: "2026-07-01T00:00:00Z"},
		{name: "this month", sel: Selector{Month: "this"}, wantFrom: "2026-06-01T00:00:00Z", wantTo: "2026-07-01T00:00:00Z"},
		{name: "previous month", sel: Selector{Month: "previous"}, wantFrom: "2026-05-01T00:00:00Z", wantTo: "2026-06-01T00:00:00Z"},
		{name: "last month", sel: Selector{Month: "LAST"}, wantFrom: "2026-05-01T00:00:00Z", wantTo: "2026-06-01T00:00:00Z"},
		{name: "day", sel: Selector{Day: "2026-06-10"}, wantFrom: "2026-06-10T00:00:00Z", wantTo: "2026-06-11T00:00:00Z"},
		{name: "explicit", sel: Selector{From: "2026-01-01", To: "2026-02-01"}, wantFrom: "2026-01-01T00:00:00Z", wantTo: "2026-02-01T00:00:00Z"},
		{name: "conflict", sel: Selector{Days: 3, Month: "2026-05"}, wantErr: true},
		{name: "bad month", sel: Selector{Month: "nope"}, wantErr: true},
		{name: "to before from", sel: Selector{From: "2026-02-01", To: "2026-01-01"}, wantErr: true},
	}

	// Tests use UTC dates; force local zone to UTC for deterministic parsing.
	time.Local = time.UTC

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.sel.Resolve(now)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.From.UTC().Format(time.RFC3339) != tc.wantFrom {
				t.Errorf("From = %s, want %s", got.From.UTC().Format(time.RFC3339), tc.wantFrom)
			}
			if got.To.UTC().Format(time.RFC3339) != tc.wantTo {
				t.Errorf("To = %s, want %s", got.To.UTC().Format(time.RFC3339), tc.wantTo)
			}
		})
	}
}
