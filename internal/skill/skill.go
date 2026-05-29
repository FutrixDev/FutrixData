// Package skill manages AI coding agent skill files and CLI binary distribution.
package skill

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
)

//go:embed templates/*
var templates embed.FS

// ---------------------------------------------------------------------------
// Agent detection & skill file management
// ---------------------------------------------------------------------------

// AgentID identifies a supported AI coding agent.
type AgentID string

const (
	AgentClaude   AgentID = "claude"
	AgentCursor   AgentID = "cursor"
	AgentCodex    AgentID = "codex"
	AgentOpenCode AgentID = "opencode"
)

const (
	SkillVersion      = "1.2.4"
	SkillMarkerPrefix = "futrixdata-skill:"
)

var legacyManagedSkillHashes = map[string]struct{}{
	"5a7c6a13719759b24b1fe7b2fcf7ff5b1e1bee7eaaac9bf63a5708f408510754": {},
	"d29f22091790865ca6536989e281b1bcf3df414c97524a58de62021b60060e33": {},
	"fd8328a50775d63794bf31923e0da810186938e76bda7063319fd4efbe77954e": {},
	"98d0d1ea1d90df4b8901be227308ca44b49d9dba49c38ef8dd2442f162a44aec": {},
	"3a4549b407b80f244054a384dc56b0823300a8c7224a77e623cec47abfa52d2f": {},
	"b0607cc17c9382829fda2f42192863d0652d7cc22c4058bb65e3d1e771044268": {},
	"d91f9aea0fa10a5431f7b9eba474e5b468c40c0d3de608c21e2fbba8cd45595c": {},
	"e6b9647905b682158a12f6b2d58cdcac321e5fff6672e547e522cc16f319705f": {},
}

var agentAccessKeyPattern = regexp.MustCompile(`--agent-access-key(?:[=\s]+)([A-Za-z0-9._-]+)`)

// Agent describes a detected AI coding agent on the user's machine.
type Agent struct {
	ID          AgentID `json:"id"`
	Name        string  `json:"name"`
	Detected    bool    `json:"detected"`
	Installed   bool    `json:"installed"`
	InstallPath string  `json:"installPath"`
	// AccessKey is the per-install key baked into the skill template, exposed
	// so the management UI can join identities by key instead of by path. The
	// backend normalizes install paths (lowercasing on macOS/Windows, symlink
	// resolution) before persisting them, and the frontend cannot reproduce
	// that transform — key-based lookups sidestep the whole class of bug.
	AccessKey   string `json:"accessKey,omitempty"`
	Version     string `json:"version,omitempty"`
	Managed     bool   `json:"managed,omitempty"`
	NeedsUpdate bool   `json:"needsUpdate,omitempty"`
}

// InstallResult is the outcome of installing/uninstalling skills.
type InstallResult struct {
	Installed []AgentInstallOutcome `json:"installed"`
}

// AgentInstallOutcome is the result for a single agent.
type AgentInstallOutcome struct {
	ID      AgentID `json:"id"`
	Name    string  `json:"name"`
	Path    string  `json:"path"`
	Success bool    `json:"success"`
	Error   string  `json:"error,omitempty"`
	// AccessKey is the per-install identity key. The install dialog uses it
	// to apply a sensitivity-classification grant chosen at install time
	// without a second round-trip to look up identities by source/path.
	AccessKey string `json:"accessKey,omitempty"`
}

type SkillInstallRequest struct {
	AgentID   AgentID
	AccessKey string
}

// AllAgentIDs returns all supported agent IDs.
func AllAgentIDs() []AgentID {
	return []AgentID{AgentClaude, AgentCursor, AgentCodex, AgentOpenCode}
}

func IsSupportedAgentID(id AgentID) bool {
	for _, candidate := range AllAgentIDs() {
		if candidate == id {
			return true
		}
	}
	return false
}

// DetectAgents scans the local machine for installed AI coding agents.
func DetectAgents() []Agent {
	return detectAgentsWithHome(homeDir())
}

func AgentDisplayName(id AgentID) string {
	name, _, _ := agentMetaWithHome(id, homeDir())
	return name
}

func detectAgentsWithHome(home string) []Agent {
	agents := make([]Agent, 0, len(AllAgentIDs()))
	for _, id := range AllAgentIDs() {
		name, detectDir, skillPath := agentMetaWithHome(id, home)
		detected := dirExists(detectDir)
		version, managed, needsUpdate, installed := skillInstallState(skillPath)
		agents = append(agents, Agent{
			ID: id, Name: name, Detected: detected,
			Installed: detected && installed, InstallPath: skillPath,
			AccessKey: boundAccessKey(skillPath),
			Version:   version, Managed: managed, NeedsUpdate: detected && needsUpdate,
		})
	}
	return agents
}

// InstallSkill writes skill files for the given agent IDs.
func InstallSkill(agentIDs []string) InstallResult {
	return installSkillWithHome(agentIDs, homeDir())
}

func installSkillWithHome(agentIDs []string, home string) InstallResult {
	requests := make([]SkillInstallRequest, 0, len(agentIDs))
	for _, raw := range agentIDs {
		requests = append(requests, SkillInstallRequest{AgentID: AgentID(strings.TrimSpace(raw))})
	}
	return installSkillRequestsWithHome(requests, home)
}

func InstallSkillRequests(requests []SkillInstallRequest) InstallResult {
	return installSkillRequestsWithHome(requests, homeDir())
}

func installSkillRequestsWithHome(requests []SkillInstallRequest, home string) InstallResult {
	var result InstallResult
	for _, request := range requests {
		id := AgentID(strings.TrimSpace(string(request.AgentID)))
		name, _, skillPath := agentMetaWithHome(id, home)
		accessKey := strings.TrimSpace(request.AccessKey)
		outcome := installOne(id, name, skillPath, accessKey)
		if outcome.Success {
			outcome.AccessKey = accessKey
		}
		result.Installed = append(result.Installed, outcome)
	}
	return result
}

func installOne(id AgentID, name, skillPath, accessKey string) AgentInstallOutcome {
	if skillPath == "" {
		return AgentInstallOutcome{ID: id, Name: name, Error: fmt.Sprintf("unknown agent: %s", id)}
	}
	content, err := renderSkillTemplate(id, accessKey)
	if err != nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: skillPath, Error: fmt.Sprintf("read template: %v", err)}
	}
	if err := os.MkdirAll(filepath.Dir(skillPath), 0755); err != nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: skillPath, Error: fmt.Sprintf("create directory: %v", err)}
	}
	if err := os.WriteFile(skillPath, content, 0644); err != nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: skillPath, Error: fmt.Sprintf("write file: %v", err)}
	}
	return AgentInstallOutcome{ID: id, Name: name, Path: skillPath, Success: true}
}

// UninstallSkill removes skill files for the given agent IDs.
func UninstallSkill(agentIDs []string) InstallResult {
	return uninstallSkillWithHome(agentIDs, homeDir())
}

func uninstallSkillWithHome(agentIDs []string, home string) InstallResult {
	var result InstallResult
	for _, raw := range agentIDs {
		id := AgentID(strings.TrimSpace(raw))
		name, _, skillPath := agentMetaWithHome(id, home)
		result.Installed = append(result.Installed, uninstallOne(id, name, skillPath))
	}
	return result
}

func uninstallOne(id AgentID, name, skillPath string) AgentInstallOutcome {
	if skillPath == "" {
		return AgentInstallOutcome{ID: id, Name: name, Error: fmt.Sprintf("unknown agent: %s", id)}
	}
	if !fileExists(skillPath) {
		return AgentInstallOutcome{ID: id, Name: name, Path: skillPath, Success: true}
	}
	if err := os.Remove(skillPath); err != nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: skillPath, Error: fmt.Sprintf("remove file: %v", err)}
	}
	dir := filepath.Dir(skillPath)
	if filepath.Base(dir) == "futrixdata" {
		_ = os.Remove(dir) // remove empty parent dir
	}
	return AgentInstallOutcome{ID: id, Name: name, Path: skillPath, Success: true}
}

// RefreshInstalledSkills updates managed FutrixData skill files when the installed
// version is older than the bundled version.
func RefreshInstalledSkills(dataPath string) InstallResult {
	return refreshInstalledSkillsWithHome(homeDir(), dataPath)
}

func refreshInstalledSkillsWithHome(home, dataPath string) InstallResult {
	var result InstallResult
	var identityStore *agentaudit.IdentityStore
	if strings.TrimSpace(dataPath) != "" {
		identityStore = agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	}
	for _, agent := range detectAgentsWithHome(home) {
		if !agent.Detected || !agent.Installed || !agent.Managed {
			continue
		}
		accessKey, needsRewrite, err := refreshedAccessKey(agent, identityStore)
		if err != nil {
			result.Installed = append(result.Installed, AgentInstallOutcome{
				ID:    agent.ID,
				Name:  agent.Name,
				Path:  agent.InstallPath,
				Error: err.Error(),
			})
			continue
		}
		if !skillNeedsRefresh(agent, needsRewrite) {
			continue
		}
		result.Installed = append(result.Installed, installOne(agent.ID, agent.Name, agent.InstallPath, accessKey))
	}
	return result
}

func skillNeedsRefresh(agent Agent, needsRewrite bool) bool {
	if agent.NeedsUpdate {
		return true
	}
	return needsRewrite
}

func refreshedAccessKey(agent Agent, identityStore *agentaudit.IdentityStore) (string, bool, error) {
	accessKey := boundAccessKey(agent.InstallPath)
	if accessKey != "" {
		if identityStore != nil {
			if _, err := identityStore.EnsureBound(accessKey, string(agent.ID), agent.Name); err != nil {
				return "", false, fmt.Errorf("ensure bound identity: %w", err)
			}
			// Rebind install path to the currently detected location whenever
			// it differs from what's stored: this covers the empty-legacy
			// backfill AND repairs a stale binding (e.g. the skill moved on
			// disk, or a normalization rule changed). BindInstallPath is
			// idempotent — it skips the write when the path is already
			// correct — and only touches the exact access key we resolved,
			// so it cannot divert to a sibling legacy row.
			if strings.TrimSpace(agent.InstallPath) != "" {
				if _, err := identityStore.BindInstallPath(accessKey, agent.InstallPath); err != nil {
					return "", false, fmt.Errorf("bind install path: %w", err)
				}
			}
		}
		return accessKey, false, nil
	}
	if identityStore == nil {
		return "", false, nil
	}
	identity, err := identityStore.EnsureForInstall(string(agent.ID), agent.InstallPath, agent.Name)
	if err != nil {
		return "", false, fmt.Errorf("ensure install identity: %w", err)
	}
	return identity.AccessKey, true, nil
}

// SkillPrompted checks if the skill install prompt has been shown before.
func SkillPrompted(dataPath string) bool {
	return fileExists(skillPromptedPath(dataPath))
}

// MarkSkillPrompted marks the skill install prompt as having been shown.
func MarkSkillPrompted(dataPath string) error {
	path := skillPromptedPath(dataPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("{\"prompted\":true}\n"), 0644)
}

func skillPromptedPath(dataPath string) string {
	dir := dataPath
	// If dataPath looks like a file (has an extension), use its parent directory.
	if ext := filepath.Ext(dir); ext != "" {
		dir = filepath.Dir(dir)
	}
	return filepath.Join(dir, "skill-prompted.json")
}

// ---------------------------------------------------------------------------
// Agent metadata & templates
// ---------------------------------------------------------------------------

func agentMetaWithHome(id AgentID, home string) (name, detectDir, skillPath string) {
	switch id {
	case AgentClaude:
		return "Claude Code", filepath.Join(home, ".claude"), filepath.Join(home, ".claude", "skills", "futrixdata", "SKILL.md")
	case AgentCursor:
		return "Cursor", filepath.Join(home, ".cursor"), filepath.Join(home, ".cursor", "rules", "futrixdata.mdc")
	case AgentCodex:
		return "Codex", filepath.Join(home, ".codex"), filepath.Join(home, ".codex", "skills", "futrixdata", "SKILL.md")
	case AgentOpenCode:
		return "OpenCode", filepath.Join(home, ".opencode"), filepath.Join(home, ".opencode", "skills", "futrixdata.md")
	default:
		return string(id), "", ""
	}
}

func templateFile(id AgentID) string {
	switch id {
	case AgentClaude:
		return "templates/claude.md"
	case AgentCursor:
		return "templates/cursor.mdc"
	case AgentCodex:
		return "templates/codex.md"
	case AgentOpenCode:
		return "templates/opencode.md"
	default:
		return ""
	}
}

func renderSkillTemplate(id AgentID, accessKey string) ([]byte, error) {
	content, err := templates.ReadFile(templateFile(id))
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(accessKey)
	flag := key
	if flag != "" {
		flag = "--agent-access-key " + flag
	}
	replaced := strings.ReplaceAll(string(content), "{{AGENT_ACCESS_KEY_FLAG}}", flag)
	replaced = strings.ReplaceAll(replaced, "{{AGENT_ACCESS_KEY}}", key)
	return []byte(replaced), nil
}

func boundAccessKey(skillPath string) string {
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return ""
	}
	matches := agentAccessKeyPattern.FindStringSubmatch(string(content))
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func skillInstallState(skillPath string) (version string, managed bool, needsUpdate bool, installed bool) {
	if !fileExists(skillPath) {
		return "", false, false, false
	}
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return "", false, false, false
	}
	version, managed = parseSkillMetadata(string(content))
	if !managed && looksLikeLegacyManagedSkill(string(content)) {
		managed = true
		version = "legacy"
	}
	if !managed {
		return version, false, false, true
	}
	return version, true, compareSkillVersions(version, SkillVersion) < 0, true
}

func parseSkillMetadata(content string) (version string, managed bool) {
	idx := strings.Index(content, SkillMarkerPrefix)
	if idx < 0 {
		return "", false
	}
	marker := content[idx+len(SkillMarkerPrefix):]
	if end := strings.Index(marker, "\n"); end >= 0 {
		marker = marker[:end]
	}
	marker = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(marker, " "), "-->"))
	for _, token := range strings.Fields(marker) {
		token = strings.TrimSpace(strings.TrimSuffix(token, "-->"))
		switch {
		case token == "managed=true":
			managed = true
		case strings.HasPrefix(token, "version="):
			version = strings.TrimSpace(strings.TrimPrefix(token, "version="))
		}
	}
	return version, managed
}

func looksLikeLegacyManagedSkill(content string) bool {
	sum := sha256.Sum256([]byte(content))
	_, ok := legacyManagedSkillHashes[hex.EncodeToString(sum[:])]
	return ok
}

func compareSkillVersions(installed, bundled string) int {
	if strings.TrimSpace(installed) == "" || strings.EqualFold(strings.TrimSpace(installed), "legacy") {
		return -1
	}
	left := parseVersionParts(installed)
	right := parseVersionParts(bundled)
	n := len(left)
	if len(right) > n {
		n = len(right)
	}
	for i := 0; i < n; i++ {
		var l, r int
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		if l < r {
			return -1
		}
		if l > r {
			return 1
		}
	}
	return 0
}

func parseVersionParts(raw string) []int {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "v"))
	if raw == "" {
		return nil
	}
	chunks := strings.Split(raw, ".")
	parts := make([]int, 0, len(chunks))
	for _, chunk := range chunks {
		value, err := strconv.Atoi(strings.TrimSpace(chunk))
		if err != nil {
			return nil
		}
		parts = append(parts, value)
	}
	return parts
}

// ---------------------------------------------------------------------------
// CLI binary distribution
// ---------------------------------------------------------------------------

// CLIInstallDir is the directory where the CLI binary is placed.
//   - macOS/Linux: ~/.local/bin  (user-writable, typically in PATH)
//   - Windows:     %LOCALAPPDATA%\FutrixData\bin
//
// Exported as a var so tests can override it.
var CLIInstallDir = defaultCLIInstallDir()

const appInstallURL = "https://futrixdata.com/"

type cliInstallMetadata struct {
	SourcePath string `json:"sourcePath,omitempty"`
	AppPath    string `json:"appPath,omitempty"`
}

type DesktopAppUnavailableError struct {
	InstallURL string
}

func (e *DesktopAppUnavailableError) Error() string {
	url := strings.TrimSpace(e.InstallURL)
	if url == "" {
		url = appInstallURL
	}
	return fmt.Sprintf("FutrixData desktop app is unavailable. Install the latest version from %s", url)
}

// CLIStatus reports the current state of the CLI binary in PATH.
type CLIStatus struct {
	InPath     bool   `json:"inPath"`
	BinaryPath string `json:"binaryPath,omitempty"`
	SymlinkTo  string `json:"symlinkTo,omitempty"` // non-empty only on Unix when installed via symlink
}

// CLIInPath checks whether futrixdata-cli is actually executable via PATH.
// Uses exec.LookPath as the authoritative check, then supplements with
// managed directory info for symlink details.
func CLIInPath() CLIStatus {
	// Primary: check if the binary is reachable via PATH.
	if found, err := exec.LookPath(cliBinName()); err == nil {
		status := CLIStatus{InPath: true, BinaryPath: found}
		if target, err := os.Readlink(found); err == nil {
			status.SymlinkTo = target
		}
		return status
	}
	// Secondary: check our managed directory (it may exist but not yet in PATH,
	// e.g. on Windows before EnsureInSystemPath runs).
	dest := cliBinaryDest()
	if fileExists(dest) {
		status := CLIStatus{InPath: false, BinaryPath: dest}
		if target, err := os.Readlink(dest); err == nil {
			status.SymlinkTo = target
		}
		return status
	}
	return CLIStatus{}
}

// InstallCLI places the CLI binary into CLIInstallDir.
// On Unix it creates a symlink; on Windows it copies the file.
// If srcPath is empty, it tries to locate the binary automatically
// (next to the running executable, then in common build paths).
func InstallCLI(srcPath string) error {
	if srcPath == "" {
		srcPath = locateCLIBinary()
	}
	if srcPath == "" {
		return fmt.Errorf("cannot locate %s binary; place it next to the desktop app or pass the path explicitly", cliBinName())
	}
	if !fileExists(srcPath) {
		return fmt.Errorf("CLI binary not found at %s", srcPath)
	}
	dest := cliBinaryDest()
	if err := os.MkdirAll(CLIInstallDir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", CLIInstallDir, err)
	}
	if runtime.GOOS == "windows" {
		if err := copyFile(srcPath, dest); err != nil {
			return err
		}
		return writeCLIInstallMetadata(cliInstallMetadata{
			SourcePath: srcPath,
			AppPath:    inferDesktopAppPathFromCLISource(srcPath),
		})
	}
	// Unix: symlink so the binary auto-updates when the app bundle is replaced.
	if target, err := os.Readlink(dest); err == nil && target == srcPath {
		return writeCLIInstallMetadata(cliInstallMetadata{
			SourcePath: srcPath,
			AppPath:    inferDesktopAppPathFromCLISource(srcPath),
		})
	}
	_ = os.Remove(dest)
	if err := os.Symlink(srcPath, dest); err != nil {
		return err
	}
	return writeCLIInstallMetadata(cliInstallMetadata{
		SourcePath: srcPath,
		AppPath:    inferDesktopAppPathFromCLISource(srcPath),
	})
}

// UninstallCLI removes the CLI binary from CLIInstallDir.
// Handles both symlinks (production install) and real binaries (--cli-only dev build).
func UninstallCLI() error {
	dest := cliBinaryDest()
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(cliInstallMetadataPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ValidateDesktopAppForCLI() error {
	exePath, err := os.Executable()
	if err != nil {
		return nil
	}
	return validateCLIExecutableAgainstDesktopApp(exePath)
}

func ValidateManagedCLIInstall() error {
	return validateCLIExecutableAgainstDesktopApp(cliBinaryDest())
}

// EnsureInSystemPath adds CLIInstallDir to the user's PATH if not already present.
// On Windows this writes to the HKCU\Environment registry key.
// On Unix this appends to shell rc files (~/.zshrc, ~/.bashrc, ~/.profile)
// to cover GUI-launched sessions where ~/.local/bin may not be in PATH.
func EnsureInSystemPath() error {
	if runtime.GOOS == "windows" {
		return addToWindowsUserPath(CLIInstallDir)
	}
	return ensureUnixShellPath(CLIInstallDir)
}

// ---------------------------------------------------------------------------
// CLI helpers
// ---------------------------------------------------------------------------

func cliBinName() string {
	if runtime.GOOS == "windows" {
		return "futrixdata-cli.exe"
	}
	return "futrixdata-cli"
}

func cliBinaryDest() string {
	return filepath.Join(CLIInstallDir, cliBinName())
}

func cliInstallMetadataPath() string {
	return filepath.Join(CLIInstallDir, "futrixdata-cli-install.json")
}

func defaultCLIInstallDir() string {
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "FutrixData", "bin")
		}
		return filepath.Join(homeDir(), "AppData", "Local", "FutrixData", "bin")
	}
	home := homeDir()
	if home == "" {
		return "/usr/local/bin"
	}
	return filepath.Join(home, ".local", "bin")
}

// locateCLIBinary tries to find the CLI binary automatically.
// If no pre-built binary exists but the source is available (dev mode),
// it compiles the CLI on-the-fly to build/bin/.
func locateCLIBinary() string {
	name := cliBinName()
	// 1. Same directory as the running executable (production: .app/Contents/MacOS/).
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if fileExists(candidate) {
			return candidate
		}
	}
	// 2. Common build paths relative to working directory.
	buildBin := "build/bin/" + name
	for _, rel := range []string{buildBin, name} {
		if abs, err := filepath.Abs(rel); err == nil && fileExists(abs) {
			return abs
		}
	}
	// 3. Dev fallback: compile from source if cmd/futrixdata-cli exists.
	if abs, err := filepath.Abs(buildBin); err == nil {
		if built := devBuildCLI(abs); built != "" {
			return built
		}
	}
	return ""
}

func validateCLIExecutableAgainstDesktopApp(exePath string) error {
	exePath = strings.TrimSpace(exePath)
	if exePath == "" {
		return nil
	}
	if !strings.EqualFold(filepath.Base(exePath), cliBinName()) {
		return nil
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil && strings.TrimSpace(resolved) != "" {
		exePath = resolved
	}
	appPath := inferDesktopAppPathFromCLISource(exePath)
	if appPath != "" {
		if pathExists(appPath) {
			return nil
		}
		return &DesktopAppUnavailableError{InstallURL: appInstallURL}
	}
	managedDest := strings.TrimSpace(cliBinaryDest())
	if managedDest == "" {
		return nil
	}
	if same, err := sameFilePath(exePath, managedDest); err != nil {
		return err
	} else if !same {
		return nil
	}
	meta, err := readCLIInstallMetadata()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &DesktopAppUnavailableError{InstallURL: appInstallURL}
		}
		return err
	}
	if strings.TrimSpace(meta.AppPath) == "" {
		return &DesktopAppUnavailableError{InstallURL: appInstallURL}
	}
	if pathExists(meta.AppPath) {
		return nil
	}
	return &DesktopAppUnavailableError{InstallURL: appInstallURL}
}

func inferDesktopAppPathFromCLISource(srcPath string) string {
	srcPath = strings.TrimSpace(srcPath)
	if srcPath == "" {
		return ""
	}
	clean := filepath.Clean(srcPath)
	if strings.HasSuffix(strings.ToLower(clean), ".app/contents/macos/"+strings.ToLower(cliBinName())) {
		macOSDir := filepath.Dir(clean)
		contentsDir := filepath.Dir(macOSDir)
		return filepath.Dir(contentsDir)
	}
	dir := filepath.Dir(clean)
	candidate := filepath.Join(dir, appBinNameForRuntime(runtime.GOOS))
	if fileExists(candidate) {
		return candidate
	}
	return ""
}

func appBinNameForRuntime(goos string) string {
	switch goos {
	case "windows":
		return "FutrixData.exe"
	default:
		return "FutrixData"
	}
}

// devBuildCLI compiles the CLI from source to dest when running in dev mode.
// Returns dest on success, empty string on failure.
func devBuildCLI(dest string) string {
	srcDir := "cmd/futrixdata-cli"
	if abs, err := filepath.Abs(srcDir); err != nil || !dirExists(abs) {
		return ""
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return ""
	}
	cmd := exec.Command("go", "build", "-trimpath", "-o", dest, "./"+srcDir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return ""
	}
	return dest
}

// ---------------------------------------------------------------------------
// Shared filesystem helpers
// ---------------------------------------------------------------------------

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	if runtime.GOOS == "windows" {
		return os.Getenv("USERPROFILE")
	}
	return ""
}

func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func sameFilePath(a, b string) (bool, error) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false, nil
	}
	aInfo, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	bInfo, err := os.Stat(b)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return os.SameFile(aInfo, bInfo), nil
}

func writeCLIInstallMetadata(meta cliInstallMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cliInstallMetadataPath(), append(data, '\n'), 0644)
}

func readCLIInstallMetadata() (cliInstallMetadata, error) {
	data, err := os.ReadFile(cliInstallMetadataPath())
	if err != nil {
		return cliInstallMetadata{}, err
	}
	var meta cliInstallMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return cliInstallMetadata{}, err
	}
	return meta, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}
