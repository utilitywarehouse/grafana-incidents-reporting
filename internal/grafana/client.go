// Package grafana is a thin wrapper around the grafana/incident-go SDK that
// exposes just the read operations this reporting tool needs.
package grafana

import (
	"context"
	"fmt"
	"strings"

	incident "github.com/grafana/incident-go"
	"github.com/utilitywarehouse/grafana-incidents-reporting/internal/timerange"
)

// pageSize is how many incidents we request per QueryIncidents call.
const pageSize = 50

// Client wraps the incident-go services used to build reports.
type Client struct {
	incidents *incident.IncidentsService
}

// New builds a Client. apiURL should be the full resources API base, e.g.
// "https://<stack>.grafana.net/api/plugins/grafana-irm-app/resources/api/v1".
//
// The underlying SDK builds request URLs by concatenating the base with
// "Service.Method" using no separator, so the base must end in a slash. We
// normalize it here so callers don't have to care.
func New(apiURL, serviceAccountToken string) *Client {
	if !strings.HasSuffix(apiURL, "/") {
		apiURL += "/"
	}
	c := incident.NewClient(apiURL, serviceAccountToken)
	return &Client{
		incidents: incident.NewIncidentsService(c),
	}
}

// QueryParams narrows which incidents are returned.
type QueryParams struct {
	Range          timerange.Range
	IncludeDrills  bool // when false, drills are excluded
	IncludeStatuses []string
}

// ListIncidents returns every incident declared within the params' time range,
// transparently paging through the API.
func (c *Client) ListIncidents(ctx context.Context, p QueryParams) ([]incident.Incident, error) {
	query := incident.IncidentsQuery{
		Limit:           pageSize,
		DateFrom:        p.Range.From.Format("2006-01-02T15:04:05Z07:00"),
		DateTo:          p.Range.To.Format("2006-01-02T15:04:05Z07:00"),
		OnlyDrills:      false,
		IncludeStatuses: p.IncludeStatuses,
		OrderDirection:  "ASC",
	}

	var (
		out    []incident.Incident
		cursor incident.Cursor
	)
	for {
		resp, err := c.incidents.QueryIncidents(ctx, incident.QueryIncidentsRequest{
			Query:  query,
			Cursor: cursor,
		})
		if err != nil {
			return nil, fmt.Errorf("query incidents: %w", err)
		}
		for _, inc := range resp.Incidents {
			if inc.IsDrill && !p.IncludeDrills {
				continue
			}
			out = append(out, inc)
		}
		if !resp.Cursor.HasMore {
			break
		}
		cursor = resp.Cursor
	}
	return out, nil
}
