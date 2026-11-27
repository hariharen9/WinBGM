package backend

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// Build-time metadata variables (overridable via -ldflags "-X winbgm/backend.AppVersion=...").
var (
	AppVersion      = "1.1.1"
	AppCommit       = ""
	AppCommitMsgB64 = ""
)

// BuildInfo exposes version and git commit details for the About UI.
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	CommitMsg string `json:"commitMsg"`
}

// GetBuildInfo returns the app version and embedded git commit metadata.
func GetBuildInfo() BuildInfo {
	commit := strings.TrimSpace(AppCommit)
	if commit == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" && s.Value != "" {
					if len(s.Value) > 7 {
						commit = s.Value[:7]
					} else {
						commit = s.Value
					}
					break
				}
			}
		}
	}
	var commitMsg string
	if raw := strings.TrimSpace(AppCommitMsgB64); raw != "" {
		if b, err := base64.StdEncoding.DecodeString(raw); err == nil {
			commitMsg = strings.TrimSpace(string(b))
		}
	}
	return BuildInfo{
		Version:   AppVersion,
		Commit:    commit,
		CommitMsg: commitMsg,
	}
}

const githubLatestReleaseURL = "https://api.github.com/repos/hariharen9/WinBGM/releases/latest"
const githubReleasesPageURL = "https://github.com/hariharen9/WinBGM/releases"

// UpdateCheckResult reports whether a newer GitHub Release is available.
type UpdateCheckResult struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseUrl"`
	ReleaseNotes    string `json:"releaseNotes"`
	PublishedAt     string `json:"publishedAt"`
	Error           string `json:"error,omitempty"`
}

// CheckForUpdates queries GitHub Releases for a newer WinBGM version.
func CheckForUpdates() UpdateCheckResult {
	res := UpdateCheckResult{
		CurrentVersion: AppVersion,
		LatestVersion:  AppVersion,
		ReleaseURL:     githubReleasesPageURL,
	}

	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest(http.MethodGet, githubLatestReleaseURL, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "WinBGM/"+AppVersion)

	resp, err := client.Do(req)
	if err != nil {
		res.Error = "Could not reach GitHub: " + err.Error()
		return res
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// No published releases yet — treat current build as up to date.
		return res
	}
	if resp.StatusCode != http.StatusOK {
		res.Error = fmt.Sprintf("GitHub API returned status %d", resp.StatusCode)
		return res
	}

	var payload struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		res.Error = "Invalid update response"
		return res
	}

	latest := strings.TrimPrefix(strings.TrimSpace(payload.TagName), "v")
	if latest != "" {
		res.LatestVersion = latest
		res.UpdateAvailable = isNewerSemver(latest, AppVersion)
	}
	if payload.HTMLURL != "" {
		res.ReleaseURL = payload.HTMLURL
	}
	res.ReleaseNotes = strings.TrimSpace(payload.Body)
	res.PublishedAt = payload.PublishedAt
	return res
}

func isNewerSemver(latest, current string) bool {
	parse := func(v string) [3]int {
		v = strings.SplitN(strings.TrimPrefix(strings.TrimSpace(v), "v"), "-", 2)[0]
		parts := strings.Split(v, ".")
		var out [3]int
		for i := 0; i < 3 && i < len(parts); i++ {
			out[i], _ = strconv.Atoi(parts[i])
		}
		return out
	}
	l := parse(latest)
	c := parse(current)
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}
