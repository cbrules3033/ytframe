//go:build windows

package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"
)

type sponsorSegment struct {
	Start    float64
	End      float64
	Category string
}

type sponsorClient struct {
	mu       sync.Mutex
	cache    map[string][]sponsorSegment
	fetched  map[string]bool
	inflight map[string]bool
	http     *http.Client
}

func newSponsorClient() *sponsorClient {
	return &sponsorClient{
		cache:    map[string][]sponsorSegment{},
		fetched:  map[string]bool{},
		inflight: map[string]bool{},
		http:     &http.Client{Timeout: 6 * time.Second},
	}
}

// cached reports whether the segments for this video have been fetched, and
// returns them if so. An empty slice with ok=true means "checked, nothing to
// skip" and avoids re-querying the API.
func (c *sponsorClient) cached(id string) ([]sponsorSegment, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.fetched[id] {
		return nil, false
	}
	return c.cache[id], true
}

func (c *sponsorClient) fetchAsync(id string) {
	c.mu.Lock()
	if c.fetched[id] || c.inflight[id] {
		c.mu.Unlock()
		return
	}
	c.inflight[id] = true
	c.mu.Unlock()

	go func() {
		segs := c.fetch(id)
		c.mu.Lock()
		c.cache[id] = segs
		c.fetched[id] = true
		delete(c.inflight, id)
		c.mu.Unlock()
	}()
}

func (c *sponsorClient) fetch(id string) []sponsorSegment {
	cats := `["sponsor","selfpromo","interaction"]`
	api := "https://sponsor.ajay.app/api/skipSegments?videoID=" +
		url.QueryEscape(id) + "&categories=" + url.QueryEscape(cats)
	req, err := http.NewRequest(http.MethodGet, api, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "YTFrame/"+appVersion)
	resp, err := c.http.Do(req)
	if err != nil {
		logf("sponsor: fetch %s failed: %v", id, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		logf("sponsor: fetch %s -> HTTP %d", id, resp.StatusCode)
		return nil
	}
	var raw []struct {
		Segment    []float64 `json:"segment"`
		Category   string    `json:"category"`
		ActionType string    `json:"actionType"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		logf("sponsor: decode %s: %v", id, err)
		return nil
	}
	out := make([]sponsorSegment, 0, len(raw))
	for _, s := range raw {
		if len(s.Segment) != 2 {
			continue
		}
		if s.ActionType != "" && s.ActionType != "skip" {
			continue
		}
		if s.Segment[1] <= s.Segment[0] {
			continue
		}
		switch s.Category {
		case "sponsor", "selfpromo", "interaction":
		default:
			continue
		}
		out = append(out, sponsorSegment{Start: s.Segment[0], End: s.Segment[1], Category: s.Category})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	if len(out) > 0 {
		logf("sponsor: %d segment(s) for %s", len(out), id)
	}
	return out
}
