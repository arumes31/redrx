package web

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/arumes31/redrx/internal/geo"
	"github.com/arumes31/redrx/internal/shortcode"
	"github.com/arumes31/redrx/internal/store"
)

// clickView decorates a stored click with the values the template shows.
type clickView struct {
	*store.Click
	RelativeTime string
	AnonymizedIP string
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	code := shortcode.Normalize(r.PathValue("code"))

	link, err := s.db.URLByShortCode(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		s.renderError(w, r, http.StatusNotFound)
		return
	}
	if err != nil {
		s.log.Error("load link for stats", "code", code, "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	// Anonymous links have public stats; owned links are private to the owner.
	if link.UserID != nil {
		user := userFrom(r)
		if user == nil || *link.UserID != user.ID {
			s.renderError(w, r, http.StatusForbidden)
			return
		}
	}

	rangeType := r.URL.Query().Get("range")
	switch rangeType {
	case "24h", "7d", "30d":
	default:
		rangeType = "30d"
	}

	now := time.Now().UTC()
	labels, cutoff, hourly := timeBuckets(rangeType, now)

	series, err := s.db.ClicksByTimeBucket(r.Context(), link.ID, cutoff, hourly)
	if err != nil {
		s.log.Error("time series stats", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	counts := make(map[string]int64, len(series))
	for _, b := range series {
		counts[b.Label] = b.Count
	}
	values := make([]int64, len(labels))
	var total int64
	for i, l := range labels {
		values[i] = counts[l]
		total += counts[l]
	}

	days := 30.0
	switch rangeType {
	case "24h":
		days = 1
	case "7d":
		days = 7
	}
	avgDaily := math.Round(float64(total)/days*10) / 10

	countryLabels, countryValues, err := s.groupedStats(r.Context(), link.ID, cutoff, "country")
	if err != nil {
		s.log.Error("country stats", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	browserLabels, browserValues, err := s.groupedStats(r.Context(), link.ID, cutoff, "browser")
	if err != nil {
		s.log.Error("browser stats", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	platformLabels, platformValues, err := s.groupedStats(r.Context(), link.ID, cutoff, "platform")
	if err != nil {
		s.log.Error("platform stats", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	referrerLabels, referrerValues, err := s.referrerStats(r.Context(), link.ID, cutoff)
	if err != nil {
		s.log.Error("referrer stats", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	recent, err := s.db.RecentClicks(r.Context(), link.ID, 10)
	if err != nil {
		s.log.Error("recent clicks", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	views := make([]clickView, 0, len(recent))
	for _, c := range recent {
		ts := c.Timestamp
		views = append(views, clickView{
			Click:        c,
			RelativeTime: timeAgo(&ts),
			// The stored value is already masked; masking again is harmless and
			// covers rows written before anonymisation existed.
			AnonymizedIP: geo.AnonymizeIP(c.IPAddress),
		})
	}

	data := s.newPageData(r)
	data.Data["url"] = link
	data.Data["short_url"] = s.cfg.ShortURL(link.ShortCode)
	data.Data["active"] = link.IsActive()
	data.Data["range_type"] = rangeType
	data.Data["avg_daily"] = avgDaily
	data.Data["time_labels"] = labels
	data.Data["time_values"] = values
	data.Data["country_labels"] = countryLabels
	data.Data["country_values"] = countryValues
	data.Data["browser_labels"] = browserLabels
	data.Data["browser_values"] = browserValues
	data.Data["platform_labels"] = platformLabels
	data.Data["platform_values"] = platformValues
	data.Data["referrer_labels"] = referrerLabels
	data.Data["referrer_values"] = referrerValues
	data.Data["recent_clicks"] = views

	s.render(w, r, http.StatusOK, "stats.html", data)
}

// timeBuckets returns the ordered bucket labels for a range, the query cutoff,
// and whether buckets are hourly.
func timeBuckets(rangeType string, now time.Time) (labels []string, cutoff time.Time, hourly bool) {
	switch rangeType {
	case "24h":
		// The cutoff is the start of the oldest labelled hour, not simply 24h
		// ago. Labels are hour-of-day only, so a cutoff mid-hour would let the
		// leading partial hour fall under the same label as the current hour --
		// one bucket holding two hours' clicks, double-counted in the total and
		// so in the daily average too.
		cutoff = now.Truncate(time.Hour).Add(-23 * time.Hour)
		for i := 23; i >= 0; i-- {
			labels = append(labels, now.Add(-time.Duration(i)*time.Hour).Format("15:00"))
		}
		return labels, cutoff, true
	case "7d":
		cutoff = now.AddDate(0, 0, -7)
		for i := 7; i >= 0; i-- {
			labels = append(labels, now.AddDate(0, 0, -i).Format("2006-01-02"))
		}
		return labels, cutoff, false
	default:
		cutoff = now.AddDate(0, 0, -30)
		for i := 30; i >= 0; i-- {
			labels = append(labels, now.AddDate(0, 0, -i).Format("2006-01-02"))
		}
		return labels, cutoff, false
	}
}

// groupedStats aggregates one categorical column, sorted by descending count so
// the busiest values lead the chart.
func (s *Server) groupedStats(ctx context.Context, urlID int64, cutoff time.Time, column string) ([]string, []int64, error) {
	buckets, err := s.db.ClicksGroupedBy(ctx, urlID, cutoff, column)
	if err != nil {
		return nil, nil, err
	}
	return sortBuckets(buckets)
}

// referrerStats groups referrers by hostname, so every path on one site counts
// together.
func (s *Server) referrerStats(ctx context.Context, urlID int64, cutoff time.Time) ([]string, []int64, error) {
	buckets, err := s.db.ClicksGroupedBy(ctx, urlID, cutoff, "referrer")
	if err != nil {
		return nil, nil, err
	}

	merged := map[string]int64{}
	for _, b := range buckets {
		merged[hostOf(b.Label)] += b.Count
	}

	out := make([]store.Bucket, 0, len(merged))
	for label, count := range merged {
		out = append(out, store.Bucket{Label: label, Count: count})
	}
	return sortBuckets(out)
}

func sortBuckets(buckets []store.Bucket) ([]string, []int64, error) {
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Count != buckets[j].Count {
			return buckets[i].Count > buckets[j].Count
		}
		return buckets[i].Label < buckets[j].Label
	})

	labels := make([]string, 0, len(buckets))
	values := make([]int64, 0, len(buckets))
	for _, b := range buckets {
		labels = append(labels, b.Label)
		values = append(values, b.Count)
	}
	return labels, values, nil
}
