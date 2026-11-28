package backend

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// Build-time metadata variables (overridable via -ldflags "-X winbgm/backend.AppVersion=...").
var (
	AppVersion      = "1.2.0"
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
	AssetName       string `json:"assetName,omitempty"`
	AssetURL        string `json:"assetUrl,omitempty"`
	AssetSize       int64  `json:"assetSize,omitempty"`
	ChecksumsURL    string `json:"checksumsUrl,omitempty"`
	IsInstaller     bool   `json:"isInstaller"`
	Error           string `json:"error,omitempty"`
}

// UpdateProgress reports download and verification progress to the frontend.
type UpdateProgress struct {
	Phase    string `json:"phase"` // checksum | download | verify | launch
	Percent  int    `json:"percent"`
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
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
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
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

	arch := "x64"
	switch runtime.GOARCH {
	case "arm64":
		arch = "arm64"
	case "386":
		arch = "x86"
	}
	var zipFallback struct {
		name string
		url  string
		size int64
	}
	for _, asset := range payload.Assets {
		low := strings.ToLower(asset.Name)
		if asset.Name == "SHA256SUMS.txt" {
			res.ChecksumsURL = asset.BrowserDownloadURL
			continue
		}
		if !strings.Contains(low, "windows-"+arch) {
			continue
		}
		if strings.HasPrefix(low, "winbgm-setup-") && strings.HasSuffix(low, ".exe") {
			res.AssetName = asset.Name
			res.AssetURL = asset.BrowserDownloadURL
			res.AssetSize = asset.Size
			res.IsInstaller = true
		} else if strings.HasSuffix(low, ".zip") && zipFallback.url == "" {
			zipFallback.name = asset.Name
			zipFallback.url = asset.BrowserDownloadURL
			zipFallback.size = asset.Size
		}
	}
	if res.AssetURL == "" && zipFallback.url != "" {
		res.AssetName = zipFallback.name
		res.AssetURL = zipFallback.url
		res.AssetSize = zipFallback.size
		res.IsInstaller = false
	}
	return res
}

func isAllowedReleaseURL(u string) bool {
	return strings.HasPrefix(u, "https://github.com/hariharen9/WinBGM/") ||
		strings.HasPrefix(u, "https://objects.githubusercontent.com/") ||
		strings.HasPrefix(u, "https://release-assets.githubusercontent.com/")
}

func fetchExpectedSHA256(ctx context.Context, checksumsURL, assetName string) (string, error) {
	if !isAllowedReleaseURL(checksumsURL) {
		return "", fmt.Errorf("refusing untrusted checksum URL: %s", checksumsURL)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "WinBGM/"+AppVersion)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum download failed with status %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 1024*1024))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && strings.EqualFold(strings.TrimPrefix(fields[1], "*"), assetName) {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksum for %s not found in SHA256SUMS.txt", assetName)
}

// DownloadAndInstallUpdate downloads the release asset, verifies its SHA-256 against SHA256SUMS.txt,
// and launches the Inno Setup installer (or reveals the portable archive in Explorer).
func DownloadAndInstallUpdate(ctx context.Context, assetURL, assetName, checksumsURL string, onProgress func(UpdateProgress)) (string, error) {
	if !isAllowedReleaseURL(assetURL) {
		return "", fmt.Errorf("refusing untrusted download URL: %s", assetURL)
	}
	safeName := filepath.Base(strings.TrimSpace(assetName))
	if safeName == "" || safeName == "." || safeName == ".." {
		return "", fmt.Errorf("invalid asset name")
	}

	var expectedHash string
	if checksumsURL != "" {
		if onProgress != nil {
			onProgress(UpdateProgress{Phase: "checksum", Percent: 0})
		}
		var err error
		expectedHash, err = fetchExpectedSHA256(ctx, checksumsURL, safeName)
		if err != nil {
			return "", err
		}
	}

	destDir := filepath.Join(os.TempDir(), "WinBGM-Update")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	destPath := filepath.Join(destDir, safeName)

	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "WinBGM/"+AppVersion)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	total := resp.ContentLength
	var received int64
	buf := make([]byte, 64*1024)
	lastEmit := time.Now()
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				out.Close()
				_ = os.Remove(destPath)
				return "", err
			}
			_, _ = hasher.Write(buf[:n])
			received += int64(n)
			if onProgress != nil && (time.Since(lastEmit) > 120*time.Millisecond || (total > 0 && received == total)) {
				pct := 0
				if total > 0 {
					pct = int((received * 100) / total)
				}
				onProgress(UpdateProgress{Phase: "download", Percent: pct, Received: received, Total: total})
				lastEmit = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			out.Close()
			_ = os.Remove(destPath)
			return "", readErr
		}
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(destPath)
		return "", err
	}

	if expectedHash != "" {
		if onProgress != nil {
			onProgress(UpdateProgress{Phase: "verify", Percent: 100, Received: received, Total: total})
		}
		actual := strings.ToLower(hex.EncodeToString(hasher.Sum(nil)))
		if actual != expectedHash {
			_ = os.Remove(destPath)
			return "", fmt.Errorf("SHA-256 mismatch: expected %s, got %s", expectedHash, actual)
		}
	}

	if strings.HasSuffix(strings.ToLower(safeName), ".exe") {
		if onProgress != nil {
			onProgress(UpdateProgress{Phase: "launch", Percent: 100, Received: received, Total: total})
		}
		cmd := exec.Command(destPath, "/SILENT", "/CLOSEAPPLICATIONS")
		if err := cmd.Start(); err != nil {
			return "", fmt.Errorf("failed to launch installer: %w", err)
		}
		return destPath, nil
	}

	_ = exec.Command("explorer.exe", "/select,", destPath).Start()
	return destPath, nil
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
