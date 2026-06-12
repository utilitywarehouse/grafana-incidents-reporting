// Package timerange resolves the various ways a user can ask for a reporting
// window into a single concrete [From, To) time range.
package timerange

import (
	"fmt"
	"time"
)

// Range is a half-open time window [From, To).
type Range struct {
	From time.Time
	To   time.Time
}

// Selector holds the mutually-flexible ways of asking for a window. Exactly one
// "shape" is expected to be set; Resolve enforces that and reports conflicts.
//
//	Days  > 0          -> the past N days, ending now
//	Month "2006-01"    -> a whole calendar month
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
		start, err := time.ParseInLocation("2006-01", s.Month, time.Local)
		if err != nil {
			return Range{}, fmt.Errorf("parse --month %q (want YYYY-MM): %w", s.Month, err)
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
