package web

import (
	"bufio"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/node"
)

type gitHourBucket struct {
	Hour         string `json:"hour"`
	LinesAdded   int    `json:"lines_added"`
	LinesRemoved int    `json:"lines_removed"`
	FilesChanged int    `json:"files_changed"`
	Commits      int    `json:"commits"`
}

type gitProductivityResponse struct {
	Since          string          `json:"since"`
	TotalCommits   int             `json:"total_commits"`
	TotalAdded     int             `json:"total_added"`
	TotalRemoved   int             `json:"total_removed"`
	TotalFiles     int             `json:"total_files"`
	HourlyBuckets  []gitHourBucket `json:"hourly_buckets"`
}

// apiGitProductivity handles GET /api/stats/git?since=24h&repo=/path
// Reads git log --numstat from the configured repo path and returns
// hourly productivity buckets for the charts.
func apiGitProductivity(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Default to CWD (the openpraxis repo) — in production this will be
		// the configured agent workspace. Can be overridden via ?repo= param.
		repoPath := r.URL.Query().Get("repo")
		if repoPath == "" {
			repoPath = "."
		}

		// Support ?hours=N, ?days=N, ?since=YYYY-MM-DD, or ?all=1
		var sinceArg string
		if r.URL.Query().Get("all") == "1" || r.URL.Query().Get("since") == "all" {
			sinceArg = "2026-01-01T00:00:00" // start of this year
		} else if s := r.URL.Query().Get("since"); s != "" {
			sinceArg = s
		} else if d := r.URL.Query().Get("days"); d != "" {
			if v, err := strconv.Atoi(d); err == nil && v > 0 {
				sinceArg = time.Now().UTC().Add(-time.Duration(v) * 24 * time.Hour).Format("2006-01-02T15:04:05")
			}
		} else {
			hours := 24
			if h := r.URL.Query().Get("hours"); h != "" {
				if v, err := strconv.Atoi(h); err == nil && v > 0 {
					hours = v
				}
			}
			sinceArg = time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format("2006-01-02T15:04:05")
		}

		// git log with numstat: emits commit header lines followed by numstat lines.
		// Format: each commit starts with "COMMIT|<hash>|<iso-timestamp>"
		// then numstat lines: "<added>\t<removed>\t<file>"
		cmd := exec.CommandContext(r.Context(), "git", "-C", repoPath,
			"log",
			"--format=COMMIT|%H|%ai",
			"--numstat",
			"--since="+sinceArg,
		)
		out, err := cmd.Output()
		if err != nil {
			// Git not available or not a repo — return empty
			writeJSON(w, gitProductivityResponse{Since: sinceArg, HourlyBuckets: []gitHourBucket{}})
			return
		}

		// Parse output into hourly buckets
		buckets := make(map[string]*gitHourBucket)
		var resp gitProductivityResponse
		var currentHour string

		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "COMMIT|") {
				// "COMMIT|<hash>|2026-05-04 07:10:41 -0400"
				parts := strings.SplitN(line, "|", 3)
				if len(parts) < 3 {
					continue
				}
				ts := strings.TrimSpace(parts[2])
				// Parse the timestamp to get the hour bucket
				t, err := time.Parse("2006-01-02 15:04:05 -0700", ts)
				if err != nil {
					t, err = time.Parse("2006-01-02 15:04:05 +0000", ts)
					if err != nil {
						continue
					}
				}
				currentHour = t.UTC().Format("2006-01-02T15:00:00Z")
				if _, ok := buckets[currentHour]; !ok {
					buckets[currentHour] = &gitHourBucket{Hour: currentHour}
				}
				buckets[currentHour].Commits++
				resp.TotalCommits++
				continue
			}

			if currentHour == "" {
				continue
			}

			// numstat line: "<added>\t<removed>\t<file>" or "-\t-\t<binary>"
			fields := strings.Split(line, "\t")
			if len(fields) < 3 {
				continue
			}
			added, err1 := strconv.Atoi(fields[0])
			removed, err2 := strconv.Atoi(fields[1])
			if err1 != nil || err2 != nil {
				continue // binary file or rename
			}
			b := buckets[currentHour]
			b.LinesAdded   += added
			b.LinesRemoved += removed
			b.FilesChanged++
			resp.TotalAdded   += added
			resp.TotalRemoved += removed
			resp.TotalFiles++
		}

		// Build padded bucket grid oldest→newest.
		// For ranges > 1 day: roll up to daily buckets so the chart spans the full history.
		// For 24h: keep hourly buckets for finer resolution.
		resp.Since = sinceArg
		now := time.Now().UTC()
		sinceTime, _ := time.Parse("2006-01-02T15:04:05", sinceArg)
		daySpan := int(now.Sub(sinceTime).Hours()/24) + 1

		if daySpan <= 1 {
			// Hourly grid for 24h view
			resp.HourlyBuckets = make([]gitHourBucket, 0, 24)
			for i := 23; i >= 0; i-- {
				h := now.Add(-time.Duration(i) * time.Hour).Format("2006-01-02T15:00:00Z")
				if b, ok := buckets[h]; ok {
					resp.HourlyBuckets = append(resp.HourlyBuckets, *b)
				} else {
					resp.HourlyBuckets = append(resp.HourlyBuckets, gitHourBucket{Hour: h})
				}
			}
		} else {
			// Daily rollup for multi-day views — merge hourly buckets into daily
			daily := make(map[string]*gitHourBucket)
			for h, b := range buckets {
				day := h[:10] + "T00:00:00Z"
				if d, ok := daily[day]; ok {
					d.LinesAdded   += b.LinesAdded
					d.LinesRemoved += b.LinesRemoved
					d.FilesChanged += b.FilesChanged
					d.Commits      += b.Commits
				} else {
					cp := *b
					cp.Hour = day
					daily[day] = &cp
				}
			}
			// Pad daily grid from sinceTime to now
			resp.HourlyBuckets = make([]gitHourBucket, 0, daySpan)
			for i := daySpan - 1; i >= 0; i-- {
				day := now.Add(-time.Duration(i) * 24 * time.Hour).Format("2006-01-02") + "T00:00:00Z"
				if b, ok := daily[day]; ok {
					resp.HourlyBuckets = append(resp.HourlyBuckets, *b)
				} else {
					resp.HourlyBuckets = append(resp.HourlyBuckets, gitHourBucket{Hour: day})
				}
			}
		}

		writeJSON(w, resp)
	}
}
