package backend

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// HeavyDependencyProfile defines known external directories and caches created by an application
// that standard uninstallers leave behind.
type HeavyDependencyProfile struct {
	Key         string   // Identifier e.g. "ollama", "android-studio"
	AppMatches  []string // Substrings to match against App ID or DisplayName (case-insensitive)
	Category    string   // e.g. "AI Models & Weights", "Developer SDKs", "Media Cache"
	Title       string   // Display title
	PathSpecs   []string // Paths with environment variable placeholders
	Description string   // Detailed explanation of what this data is
}

var knownHeavyProfiles = []HeavyDependencyProfile{
	{
		Key:         "ollama",
		AppMatches:  []string{"ollama"},
		Category:    "AI Models & Weights",
		Title:       "Ollama Downloaded Model Weights",
		PathSpecs:   []string{`%USERPROFILE%\.ollama\models`},
		Description: "Trained LLM model weight blobs (GGUF, safetensors, manifests). Can be tens or hundreds of gigabytes.",
	},
	{
		Key:         "lm-studio",
		AppMatches:  []string{"lm studio", "lm-studio", "lmstudio"},
		Category:    "AI Models & Weights",
		Title:       "LM Studio Model Repository",
		PathSpecs:   []string{`%USERPROFILE%\.cache\lm-studio\models`},
		Description: "Hugging Face and local GGUF model files downloaded via LM Studio.",
	},
	{
		Key:         "jan",
		AppMatches:  []string{"jan"},
		Category:    "AI Models & Weights",
		Title:       "Jan.ai Model Storage",
		PathSpecs:   []string{`%USERPROFILE%\jan\models`},
		Description: "Downloaded AI models, vector embeddings, and chat history.",
	},
	{
		Key:         "comfyui",
		AppMatches:  []string{"comfy", "comfyui", "comfy desktop"},
		Category:    "AI Models & Weights",
		Title:       "ComfyUI Checkpoints & LoRAs",
		PathSpecs:   []string{`%USERPROFILE%\ComfyUI\models`, `%USERPROFILE%\.comfyui`},
		Description: "Stable Diffusion checkpoints, UNET models, VAEs, and custom node cache.",
	},
	{
		Key:         "android-studio",
		AppMatches:  []string{"android studio", "android-studio"},
		Category:    "Developer SDK & Emulators",
		Title:       "Android Virtual Devices & SDK Platform",
		PathSpecs:   []string{`%USERPROFILE%\.android\avd`, `%LOCALAPPDATA%\Android\Sdk`, `%USERPROFILE%\.gradle\caches`},
		Description: "Android Virtual Device (AVD) system disk images, platform packages, and build caches.",
	},
	{
		Key:         "vscode",
		AppMatches:  []string{"visual studio code", "vscode"},
		Category:    "IDE Extensions & Runtimes",
		Title:       "VS Code Extensions & Language Servers",
		PathSpecs:   []string{`%USERPROFILE%\.vscode\extensions`, `%APPDATA%\Code\CachedData`, `%APPDATA%\Code\User\workspaceStorage`},
		Description: "Installed extension packages, node/python language server binaries, and workspace caches.",
	},
	{
		Key:         "cursor",
		AppMatches:  []string{"cursor"},
		Category:    "IDE Extensions & Runtimes",
		Title:       "Cursor AI Extensions & Workspace Cache",
		PathSpecs:   []string{`%USERPROFILE%\.cursor\extensions`, `%APPDATA%\Cursor\CachedData`, `%APPDATA%\Cursor\User\workspaceStorage`},
		Description: "Cursor editor plugins, code indexing databases, and language server binaries.",
	},
	{
		Key:         "windsurf",
		AppMatches:  []string{"windsurf"},
		Category:    "IDE Extensions & Runtimes",
		Title:       "Windsurf Extensions & Cache",
		PathSpecs:   []string{`%USERPROFILE%\.windsurf\extensions`, `%APPDATA%\Windsurf\CachedData`},
		Description: "Windsurf editor plugins and language server binaries.",
	},
	{
		Key:         "capcut",
		AppMatches:  []string{"capcut"},
		Category:    "Media Cache & Drafts",
		Title:       "CapCut Render Cache & Material Downloads",
		PathSpecs:   []string{`%LOCALAPPDATA%\CapCut\User Data\Cache`, `%LOCALAPPDATA%\CapCut\User Data\Projects`},
		Description: "Downloaded fonts, stickers, audio effects, render cache, and video project drafts.",
	},
	{
		Key:         "adobe-creative",
		AppMatches:  []string{"adobe premiere", "premiere pro", "after effects", "media encoder"},
		Category:    "Media Cache",
		Title:       "Adobe Common Media Cache & Peak Files",
		PathSpecs:   []string{`%APPDATA%\Adobe\Common\Media Cache Files`, `%APPDATA%\Adobe\Common\Media Cache`},
		Description: "Conformed audio, video index files, and scratch media buffers.",
	},
	{
		Key:         "docker",
		AppMatches:  []string{"docker desktop", "docker"},
		Category:    "Virtual Disks & Containers",
		Title:       "Docker WSL2 Virtual Hard Disks",
		PathSpecs:   []string{`%LOCALAPPDATA%\Docker\wsl`},
		Description: "WSL2 ext4 virtual hard disk (.vhdx) containing all downloaded docker images and containers.",
	},
	{
		Key:         "spotify",
		AppMatches:  []string{"spotify"},
		Category:    "Media Cache",
		Title:       "Spotify Offline Storage & Audio Cache",
		PathSpecs:   []string{`%LOCALAPPDATA%\Spotify\Storage`},
		Description: "Encrypted downloaded songs for offline playback and persistent streaming cache.",
	},
	{
		Key:         "discord",
		AppMatches:  []string{"discord"},
		Category:    "App Cache",
		Title:       "Discord Chromium & Code Cache",
		PathSpecs:   []string{`%APPDATA%\discord\Cache`, `%APPDATA%\discord\Code Cache`},
		Description: "Cached media, server emotes, voice logs, and Electron V8 compilation cache.",
	},
	{
		Key:         "node-npm",
		AppMatches:  []string{"node.js", "nodejs", "npm"},
		Category:    "Package Manager Cache",
		Title:       "npm Global Cache & Packages",
		PathSpecs:   []string{`%LOCALAPPDATA%\npm-cache`, `%APPDATA%\npm`},
		Description: "Tarballs and binary packages downloaded during npm install operations.",
	},
	{
		Key:         "rust-cargo",
		AppMatches:  []string{"rust", "cargo"},
		Category:    "Package Manager Cache",
		Title:       "Cargo Crate Cache & Rust Toolchains",
		PathSpecs:   []string{`%USERPROFILE%\.cargo\registry\cache`, `%USERPROFILE%\.cargo\bin`},
		Description: "Compressed source crates and compiled rust toolchains.",
	},
}

// FindMatchingProfiles finds any heavy dependency profile matching an app name or id.
func FindMatchingProfiles(name, id string) []HeavyDependencyProfile {
	lowName := strings.ToLower(name)
	lowID := strings.ToLower(id)

	var matched []HeavyDependencyProfile
	seen := make(map[string]bool)

	for _, prof := range knownHeavyProfiles {
		if seen[prof.Key] {
			continue
		}
		for _, m := range prof.AppMatches {
			if strings.Contains(lowName, m) || strings.Contains(lowID, m) {
				matched = append(matched, prof)
				seen[prof.Key] = true
				break
			}
		}
	}
	return matched
}

var envRegex = regexp.MustCompile(`%([^%]+)%`)

// ExpandPath expands Windows environment variables like %USERPROFILE%, %APPDATA%, %LOCALAPPDATA%.
func ExpandPath(path string) string {
	return envRegex.ReplaceAllStringFunc(path, func(m string) string {
		varName := m[1 : len(m)-1]
		val := os.Getenv(varName)
		if val != "" {
			return val
		}
		return m
	})
}

// ComputeFolderSize calculates total bytes and file count for a directory.
// Includes a maxFiles and depth safeguard to prevent hanging on massive hierarchies.
func ComputeFolderSize(dir string, maxFiles int) (uint64, int) {
	if maxFiles <= 0 {
		maxFiles = 3000
	}
	var totalBytes uint64
	var count int

	clean := filepath.Clean(dir)
	if len(clean) <= 3 || strings.EqualFold(clean, os.Getenv("SystemRoot")) ||
		strings.EqualFold(clean, os.Getenv("ProgramFiles")) ||
		strings.EqualFold(clean, os.Getenv("ProgramFiles(x86)")) {
		return 0, 0
	}

	baseDepth := strings.Count(clean, string(os.PathSeparator))

	_ = filepath.Walk(clean, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			currentDepth := strings.Count(p, string(os.PathSeparator))
			if currentDepth-baseDepth > 4 {
				return filepath.SkipDir
			}
			return nil
		}
		totalBytes += uint64(info.Size())
		count++
		if count >= maxFiles {
			return filepath.SkipDir
		}
		return nil
	})

	return totalBytes, count
}
