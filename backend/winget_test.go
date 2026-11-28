package backend

import (
	"strings"
	"testing"
)

const wingetSampleOutput = `Name                                                         Id                                    Version              Available           Source
---------------------------------------------------------------------------------------------------------------------------------------------------
7-Zip 26.01 (x64)                                            7zip.7zip                             26.01                26.06               winget
Claude Code                                                  Anthropic.ClaudeCode                  2.1.292              2.1.332             winget
Go Programming Language amd64 go1.26.3                       GoLang.Go                             1.26.3               1.27.4              winget
Microsoft Visual C++ v14 Redistributable (x86) - 14.51.36247 Microsoft.VCRedist.2015+.x86          14.51.36247.0        14.52.37029.0       winget
Microsoft Teams                                              Microsoft.Teams                       26106.1906.4665.7308 26304.1207.5384.858 winget
Filee                                                        XPDCN05L89WS0C                        2.2.0                2.5.0               msstore
Some Pinned App                                              Vendor.PinnedApp                      1.0.0                Blocked             winget
Mystery Package                                              Vendor.Mystery                        1.0.0                Unknown             winget
2 upgrades available.
A newer version of winget is available.
`

func TestParseWingetUpgradesHeaderOffsets(t *testing.T) {
	upd := parseWingetUpgrades(wingetSampleOutput)
	got := map[string]WingetUpdate{}
	for _, u := range upd {
		got[u.ID] = u
	}

	if len(upd) != 8 {
		t.Fatalf("expected 8 updates, got %d: %+v", len(upd), upd)
	}

	if u := got["7zip.7zip"]; u.Name != "7-Zip 26.01 (x64)" || u.CurrentVersion != "26.01" || u.AvailableVersion != "26.06" || u.Source != "winget" {
		t.Errorf("7zip parse wrong: %+v", u)
	}
	// Name with internal single spaces must survive.
	if u := got["GoLang.Go"]; u.Name != "Go Programming Language amd64 go1.26.3" {
		t.Errorf("GoLang name parse wrong: %+v", u)
	}
	// ID with '+' and long names that touch the Id column boundary.
	if u := got["Microsoft.VCRedist.2015+.x86"]; u.CurrentVersion != "14.51.36247.0" || u.AvailableVersion != "14.52.37029.0" {
		t.Errorf("VCRedist parse wrong: %+v", u)
	}
	// Very long version numbers filling the whole column width.
	if u := got["Microsoft.Teams"]; u.CurrentVersion != "26106.1906.4665.7308" || u.AvailableVersion != "26304.1207.5384.858" {
		t.Errorf("Teams parse wrong: %+v", u)
	}
	// msstore source id.
	if u := got["XPDCN05L89WS0C"]; u.Source != "msstore" {
		t.Errorf("msstore source parse wrong: %+v", u)
	}
	// Blocked => pinned, Unknown => unknownAvailable.
	if u := got["Vendor.PinnedApp"]; !u.Pinned || u.UnknownAvailable {
		t.Errorf("pinned flags wrong: %+v", u)
	}
	if u := got["Vendor.Mystery"]; !u.UnknownAvailable || u.Pinned {
		t.Errorf("unknown flags wrong: %+v", u)
	}
}

func TestParseWingetUpgradesIgnoresNoise(t *testing.T) {
	upd := parseWingetUpgrades(wingetSampleOutput)
	for _, u := range upd {
		if strings.Contains(u.ID, " ") || strings.Contains(u.Name, "upgrades available") {
			t.Errorf("noise leaked into results: %+v", u)
		}
	}
	if len(upd) != 8 { // header + separator + 2 footer lines filtered
		t.Fatalf("count changed after noise filtering: %d", len(upd))
	}
}

func TestParseWingetUpgradesFallbackSplitting(t *testing.T) {
	// Simulated localized output: no recognizable English header.
	localized := strings.Join([]string{
		"„Name“                                            „ID“                    „Version“         „Verfügbar“        „Quelle“",
		"---------------------------------------------------------------------------",
		"Mozilla Firefox                                   Mozilla.Firefox         120.0             121.0              winget",
		"VLC media player                                  VideoLAN.VLC            3.0.20            3.0.21             winget",
		"",
		"2 Pakete haben ein Upgrade verfügbar.",
	}, "\n")
	upd := parseWingetUpgrades(localized)
	if len(upd) != 2 {
		t.Fatalf("fallback parse expected 2, got %d: %+v", len(upd), upd)
	}
	if upd[0].ID != "Mozilla.Firefox" || upd[0].Name != "Mozilla Firefox" || upd[0].CurrentVersion != "120.0" || upd[0].AvailableVersion != "121.0" || upd[0].Source != "winget" {
		t.Errorf("fallback row 1 wrong: %+v", upd[0])
	}
	if upd[1].ID != "VideoLAN.VLC" {
		t.Errorf("fallback row 2 wrong: %+v", upd[1])
	}
}

func TestParseWingetUpgradesEmptyAndDegenerate(t *testing.T) {
	if upd := parseWingetUpgrades(""); len(upd) != 0 {
		t.Errorf("empty input should give 0 updates, got %d", len(upd))
	}
	noUpgrades := "Name  Id  Version  Available  Source\n------------------------------------------------------------------\nNo available upgrades.\n"
	if upd := parseWingetUpgrades(noUpgrades); len(upd) != 0 {
		t.Errorf("no-upgrade output should give 0 updates, got %d: %+v", len(upd), upd)
	}
	if upd := parseWingetUpgrades("garbage line\nanother garbage"); len(upd) != 0 {
		t.Errorf("garbage should give 0 updates, got %d: %+v", len(upd), upd)
	}
}

func TestParseWingetUpgradesDedupesByID(t *testing.T) {
	dup := "Name      Id        Version   Available   Source\n-----\nApp One   Foo.Bar   1.0       2.0         winget\nApp Two   Foo.Bar   1.0       2.0         winget\n"
	upd := parseWingetUpgrades(dup)
	if len(upd) != 1 {
		t.Fatalf("expected dedupe to 1, got %d", len(upd))
	}
}

func TestWingetIDRe(t *testing.T) {
	valid := []string{"Git.Git", "Microsoft.VCRedist.2015+.x86", "XPDCN05L89WS0C", "9NBLGGH4NNS1", "VideoLAN.VLC", "a"}
	invalid := []string{"", "two words", "Foo;Bar", "Foo&del", "-leading-dash", strings.Repeat("a", 200), "C:\\path\\exe"}
	for _, id := range valid {
		if !wingetIDRe.MatchString(id) {
			t.Errorf("id %q should be valid", id)
		}
	}
	for _, id := range invalid {
		if wingetIDRe.MatchString(id) {
			t.Errorf("id %q should be invalid", id)
		}
	}
}

func TestWingetVersionNoteAndSanitize(t *testing.T) {
	if s := wingetVersionNote(WingetUpdate{CurrentVersion: "1.0", AvailableVersion: "2.0"}); s != " (1.0 → 2.0)" {
		t.Errorf("version note wrong: %q", s)
	}
	if s := wingetVersionNote(WingetUpdate{CurrentVersion: "1.0", AvailableVersion: "Unknown", UnknownAvailable: true}); s != "" {
		t.Errorf("unknown version note should be empty, got %q", s)
	}
	if s := sanitizeHistoryKey("Microsoft.VCRedist.2015+.x86"); s != "microsoft.vcredist.2015_.x86" {
		t.Errorf("sanitize wrong: %q", s)
	}
}

func TestIsWingetNoiseLine(t *testing.T) {
	noise := []string{
		"----------",
		"- \\ | / progress",
		"2 upgrades available.",
		"The following packages have an upgrade available, but require explicit targeting for upgrade:",
		"A newer version of winget is available.",
	}
	for _, n := range noise {
		if !isWingetNoiseLine(n) {
			t.Errorf("%q should be noise", n)
		}
	}
	if isWingetNoiseLine("7-Zip 26.01 (x64)   7zip.7zip   26.01   26.06   winget") {
		t.Errorf("data line should not be noise")
	}
}

func TestIndexWord(t *testing.T) {
	if got := indexWord("Name   Id   Version   Available   Source", "Id"); got != 7 {
		t.Errorf("indexWord basic wrong: %d", got)
	}
	if got := indexWord("Builder Id Version", "Id"); got != 8 {
		t.Errorf("indexWord should skip embedded Id in Builder: %d", got)
	}
	if got := indexWord("Id Version", "Id"); got != 0 {
		t.Errorf("indexWord at start wrong: %d", got)
	}
	if got := indexWord("Version Available", "Id"); got != -1 {
		t.Errorf("indexWord missing should be -1: %d", got)
	}
}

// TestParseWingetUpgradesLive validates the parser against the real winget
// output on machines that have it; skipped elsewhere.
func TestParseWingetUpgradesLive(t *testing.T) {
	exe, err := wingetPath()
	if err != nil {
		t.Skip("winget not installed on this machine")
	}
	out, err := wingetRun(exe, wingetScanTimeout,
		"upgrade", "--include-unknown", "--accept-source-agreements", "--disable-interactivity", "--nowarn")
	if err != nil && out == "" {
		t.Skipf("winget upgrade failed to run: %v", err)
	}
	upd := parseWingetUpgrades(out)
	t.Logf("live winget reported %d upgradable package(s)", len(upd))
	for _, u := range upd {
		if u.ID == "" || u.Name == "" || u.CurrentVersion == "" || u.AvailableVersion == "" {
			t.Errorf("incomplete row parsed from live output: %+v", u)
		}
		if u.UnknownAvailable && u.AvailableVersion != "Unknown" {
			t.Errorf("unknownAvailable without Unknown literal: %+v", u)
		}
	}
}
