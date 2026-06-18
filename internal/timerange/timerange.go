// Package timerange resolves the various ways a user can ask for a reporting
// window into a single concrete [From, To) time range.
package timerange

import (
	"fmt"
	"strings"
	"time"
)

// Range is a half-open time window [From, To).
type Range struct {
	From time.Time
	To   time.Time
}

// Slug returns a deterministic, filename-safe label for the window, derived
// purely from the concrete range so the same window always yields the same
// name regardless of how it was requested:
//
//	a whole calendar month  -> "2006-01"
//	a single calendar day   -> "2006-01-02"
//	anything else           -> "2006-01-02_2006-01-02" (From and To dates)
func (r Range) Slug() string {
	if isMidnight(r.From) {
		if r.From.Day() == 1 && r.To.Equal(r.From.AddDate(0, 1, 0)) {
			return r.From.Format("2006-01")
		}
		if r.To.Equal(r.From.AddDate(0, 0, 1)) {
			return r.From.Format("2006-01-02")
		}
	}
	return r.From.Format("2006-01-02") + "_" + r.To.Format("2006-01-02")
}

// isMidnight reports whether t falls exactly on a midnight boundary.
func isMidnight(t time.Time) bool {
	h, m, s := t.Clock()
	return h == 0 && m == 0 && s == 0 && t.Nanosecond() == 0
}

// Selector holds the mutually-flexible ways of asking for a window. Exactly one
// "shape" is expected to be set; Resolve enforces that and reports conflicts.
//
//	Days  > 0          -> the past N days, ending now
//	Month "2006-01"    -> a whole calendar month; also "current"/"this" or
//	                      "previous"/"last" for the month relative to now
//	Day   "2006-01-02" -> a single calendar day
//	From/To            -> an explicit range (RFC3339 or YYYY-MM-DD)
type Selector struct {
	Days  int
	Month string
	Day   string
	From  string
	To    string
}

// Resolve turns a Selector into a concrete Range relative to now. All-zero
// selectors default to the past 7 days.
func (s Selector) Resolve(now time.Time) (Range, error) {
	switch {
	case s.set(s.Month != "", s.Day != "", s.From != "" || s.To != "", s.Days > 0) > 1:
		return Range{}, fmt.Errorf("choose only one of --days, --month, --day, or --from/--to")

	case s.Month != "":
		start, err := parseMonth(s.Month, now)
		if err != nil {
			return Range{}, err
		}
		return Range{From: start, To: start.AddDate(0, 1, 0)}, nil

	case s.Day != "":
		start, err := time.ParseInLocation("2006-01-02", s.Day, time.Local)
		if err != nil {
			return Range{}, fmt.Errorf("parse --day %q (want YYYY-MM-DD): %w", s.Day, err)
		}
		return Range{From: start, To: start.AddDate(0, 0, 1)}, nil

	case s.From != "" || s.To != "":
		r := Range{To: now}
		if s.From != "" {
			t, err := parseTime(s.From)
			if err != nil {
				return Range{}, fmt.Errorf("parse --from: %w", err)
			}
			r.From = t
		}
		if s.To != "" {
			t, err := parseTime(s.To)
			if err != nil {
				return Range{}, fmt.Errorf("parse --to: %w", err)
			}
			r.To = t
		}
		if !r.From.IsZero() && r.To.Before(r.From) {
			return Range{}, fmt.Errorf("--to (%s) is before --from (%s)", r.To.Format(time.RFC3339), r.From.Format(time.RFC3339))
		}
		return r, nil

	default:
		days := s.Days
		if days <= 0 {
			days = 7
		}
		return Range{From: now.AddDate(0, 0, -days), To: now}, nil
	}
}

// parseMonth resolves a --month value to the first instant of that calendar
// month in the local zone. It accepts an explicit "YYYY-MM" or the relative
// keywords "current"/"this" and "previous"/"last" (relative to now).
func parseMonth(s string, now time.Time) (time.Time, error) {
	firstOfMonth := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "current", "this":
		return firstOfMonth(now), nil
	case "previous", "last":
		return firstOfMonth(now).AddDate(0, -1, 0), nil
	}
	start, err := time.ParseInLocation("2006-01", s, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse --month %q (want YYYY-MM, \"current\", or \"previous\"): %w", s, err)
	}
	return start, nil
}

// set counts how many of the given conditions are true.
func (Selector) set(conds ...bool) int {
	n := 0
	for _, c := range conds {
		if c {
			n++
		}
	}
	return n
}

// parseTime accepts RFC3339 timestamps or bare YYYY-MM-DD dates (interpreted at
// local midnight).
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q (want RFC3339 or YYYY-MM-DD)", s)
}
