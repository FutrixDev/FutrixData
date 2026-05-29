package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
)

func TestDetectAgents(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)
	os.MkdirAll(filepath.Join(home, ".cursor"), 0755)

	agents := detectAgentsWithHome(home)
	if len(agents) != 4 {
		t.Fatalf("expected 4 agents, got %d", len(agents))
	}
	byID := map[AgentID]Agent{}
	for _, a := range agents {
		byID[a.ID] = a
	}
	if !byID[AgentClaude].Detected {
		t.Error("expected Claude detected")
	}
	if byID[AgentClaude].Installed {
		t.Error("expected Claude not installed yet")
	}
	if !byID[AgentCursor].Detected {
		t.Error("expected Cursor detected")
	}
	if byID[AgentCodex].Detected {
		t.Error("expected Codex not detected")
	}
	if byID[AgentOpenCode].Detected {
		t.Error("expected OpenCode not detected")
	}
}

func TestInstallSkill(t *testing.T) {
	home := t.TempDir()
	result := installSkillWithHome([]string{"claude", "cursor", "codex", "opencode"}, home)
	if len(result.Installed) != 4 {
		t.Fatalf("expected 4 outcomes, got %d", len(result.Installed))
	}
	for _, o := range result.Installed {
		if !o.Success {
			t.Errorf("install %s failed: %s", o.ID, o.Error)
			continue
		}
		data, err := os.ReadFile(o.Path)
		if err != nil {
			t.Errorf("read %s: %v", o.ID, err)
			continue
		}
		if !strings.Contains(string(data), "futrixdata-cli") {
			t.Errorf("%s skill missing futrixdata-cli reference", o.ID)
		}
	}
	// Claude should be directory/SKILL.md with YAML frontmatter.
	claudePath := filepath.Join(home, ".claude", "skills", "futrixdata", "SKILL.md")
	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("Claude skill not at expected path %s: %v", claudePath, err)
	}
	if !strings.HasPrefix(string(data), "---\n") {
		t.Error("Claude skill should have YAML frontmatter")
	}
	// Codex should also be directory/SKILL.md.
	codexPath := filepath.Join(home, ".codex", "skills", "futrixdata", "SKILL.md")
	if _, err := os.ReadFile(codexPath); err != nil {
		t.Errorf("Codex skill not at expected path %s: %v", codexPath, err)
	}
	// Cursor should have alwaysApply.
	data, _ = os.ReadFile(filepath.Join(home, ".cursor", "rules", "futrixdata.mdc"))
	if !strings.Contains(string(data), "alwaysApply: true") {
		t.Error("Cursor skill should contain alwaysApply")
	}
	// Re-detect should show installed.
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)
	os.MkdirAll(filepath.Join(home, ".cursor"), 0755)
	os.MkdirAll(filepath.Join(home, ".codex"), 0755)
	os.MkdirAll(filepath.Join(home, ".opencode"), 0755)
	for _, a := range detectAgentsWithHome(home) {
		if !a.Installed {
			t.Errorf("expected %s to show installed after install", a.ID)
		}
		if a.Version != SkillVersion {
			t.Errorf("expected %s to report version %q, got %q", a.ID, SkillVersion, a.Version)
		}
	}
}

func TestInstallSkillRequestsInjectsAccessKey(t *testing.T) {
	home := t.TempDir()
	result := installSkillRequestsWithHome([]SkillInstallRequest{{AgentID: AgentClaude, AccessKey: "agent_test_1234"}}, home)
	if len(result.Installed) != 1 || !result.Installed[0].Success {
		t.Fatalf("install failed: %+v", result.Installed)
	}
	data, err := os.ReadFile(result.Installed[0].Path)
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	if !strings.Contains(string(data), "--agent-access-key agent_test_1234") {
		t.Fatalf("expected access key flag in rendered skill, got %s", string(data))
	}
}

func TestUninstallSkill(t *testing.T) {
	home := t.TempDir()
	installSkillWithHome([]string{"claude", "codex"}, home)
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)
	os.MkdirAll(filepath.Join(home, ".codex"), 0755)

	for _, a := range detectAgentsWithHome(home) {
		if (a.ID == AgentClaude || a.ID == AgentCodex) && !a.Installed {
			t.Fatalf("expected %s installed before uninstall", a.ID)
		}
	}

	result := uninstallSkillWithHome([]string{"claude", "codex"}, home)
	for _, o := range result.Installed {
		if !o.Success {
			t.Errorf("uninstall %s failed: %s", o.ID, o.Error)
		}
	}
	for _, a := range detectAgentsWithHome(home) {
		if (a.ID == AgentClaude || a.ID == AgentCodex) && a.Installed {
			t.Errorf("expected %s uninstalled", a.ID)
		}
	}
}

func TestInstallSkillUnknown(t *testing.T) {
	result := InstallSkill([]string{"nonexistent"})
	if len(result.Installed) != 1 || result.Installed[0].Success {
		t.Error("expected failure for unknown agent")
	}
}

func TestSkillPrompted(t *testing.T) {
	dir := t.TempDir()
	if SkillPrompted(dir) {
		t.Error("should not be prompted initially")
	}
	if err := MarkSkillPrompted(dir); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if !SkillPrompted(dir) {
		t.Error("should be prompted after marking")
	}
	// Also works with json file path.
	dir2 := t.TempDir()
	if err := MarkSkillPrompted(filepath.Join(dir2, "datasources.json")); err != nil {
		t.Fatalf("mark json: %v", err)
	}
	if !SkillPrompted(filepath.Join(dir2, "datasources.json")) {
		t.Error("should be prompted with json path")
	}
}

func TestTemplatesEmbedded(t *testing.T) {
	for _, id := range AllAgentIDs() {
		data, err := templates.ReadFile(templateFile(id))
		if err != nil {
			t.Errorf("read template %s: %v", id, err)
		} else if len(data) == 0 {
			t.Errorf("template %s is empty", id)
		}
	}
}

func TestSkillContentEnglishOnly(t *testing.T) {
	for _, id := range AllAgentIDs() {
		data, err := templates.ReadFile(templateFile(id))
		if err != nil {
			t.Errorf("read template %s: %v", id, err)
			continue
		}
		content := string(data)
		if strings.Contains(content, "中文") || strings.Contains(content, "安装技能") || strings.Contains(content, "故障排查") {
			t.Errorf("%s skill should be English-only", id)
		}
		if !strings.Contains(content, SkillMarkerPrefix) {
			t.Errorf("%s skill should contain managed marker", id)
		}
		if !strings.Contains(content, "version="+SkillVersion) {
			t.Errorf("%s skill should contain version marker", id)
		}
	}
}

func TestTemplatesIncludeRiskActionSemantics(t *testing.T) {
	required := []string{
		"### Risk Action Semantics",
		"`allow`",
		"`warn`",
		"`require_approval`",
		"`block`",
		"`approvalRequired`",
		"`approvalRequired.riskAttribution`",
		"waiting for the user to approve it in FutrixData",
		"Do not pass `--approve`, add an `approve` parameter, or self-approve",
	}
	for _, id := range AllAgentIDs() {
		data, err := templates.ReadFile(templateFile(id))
		if err != nil {
			t.Errorf("read template %s: %v", id, err)
			continue
		}
		content := string(data)
		for _, want := range required {
			if !strings.Contains(content, want) {
				t.Errorf("%s skill missing risk semantics text %q", id, want)
			}
		}
		if strings.Contains(content, "`deny`") || strings.Contains(content, "`info`") {
			t.Errorf("%s skill must not introduce deny/info as canonical risk actions", id)
		}
	}
}

func TestTemplatesIncludeAgentAccessModel(t *testing.T) {
	required := []string{
		"## Agent Access Model",
		"not a separate FutrixData user account",
		"inherits the local user's configured datasource visibility",
		"datasource allowlist",
		"broad datasource listing",
		"do not expire automatically by default",
		"expired calls are rejected",
		"agent calls cannot self-approve",
		"Datasource creation, sensitivity-policy writes, and risk-rule writes also have explicit per-agent grants",
		"Audit statuses distinguish successful, approval-required, forbidden, revoked, and expired attempts",
	}
	for _, id := range AllAgentIDs() {
		data, err := templates.ReadFile(templateFile(id))
		if err != nil {
			t.Errorf("read template %s: %v", id, err)
			continue
		}
		content := string(data)
		for _, want := range required {
			if !strings.Contains(content, want) {
				t.Errorf("%s skill missing agent access model text %q", id, want)
			}
		}
	}
}

func TestTemplatesIncludeAddDatasourceWorkflow(t *testing.T) {
	required := []string{
		"## Adding A Datasource",
		"`add_datasource`",
		"`create_datasource` remains available",
		"futrixdata-cli tool describe add_datasource --json",
		"futrixdata-cli tool call test_datasource_payload --stdin --json",
		"futrixdata-cli tool call add_datasource --stdin --json",
		"datasource-management grant",
		"third-party agents cannot approve FutrixData operations",
		"must not set `options.trustLevel` to `trusted` or `danger`",
	}
	for _, id := range AllAgentIDs() {
		data, err := templates.ReadFile(templateFile(id))
		if err != nil {
			t.Errorf("read template %s: %v", id, err)
			continue
		}
		content := string(data)
		for _, want := range required {
			if !strings.Contains(content, want) {
				t.Errorf("%s skill missing add datasource workflow text %q", id, want)
			}
		}
	}
}

func TestDetectAgentsReportsOutdatedManagedSkill(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills", "futrixdata"), 0o755); err != nil {
		t.Fatalf("mkdir claude dir: %v", err)
	}
	path := filepath.Join(home, ".claude", "skills", "futrixdata", "SKILL.md")
	content := strings.ReplaceAll(string(mustReadTemplate(t, AgentClaude)), "version="+SkillVersion, "version=0.9.0")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write outdated skill: %v", err)
	}

	claude := findAgent(detectAgentsWithHome(home), AgentClaude)
	if !claude.Installed {
		t.Fatal("expected claude skill to be detected as installed")
	}
	if !claude.NeedsUpdate {
		t.Fatal("expected claude skill to require update")
	}
	if claude.Version != "0.9.0" {
		t.Fatalf("version = %q, want 0.9.0", claude.Version)
	}
}

func TestDetectAgentsReportsUnmanagedSkillAsInstalled(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex", "skills", "futrixdata"), 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	path := filepath.Join(home, ".codex", "skills", "futrixdata", "SKILL.md")
	content := "# custom skill\nUse my local instructions.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write unmanaged skill: %v", err)
	}

	codex := findAgent(detectAgentsWithHome(home), AgentCodex)
	if !codex.Installed {
		t.Fatal("expected unmanaged codex skill to be detected as installed")
	}
	if codex.Managed {
		t.Fatal("expected unmanaged codex skill to stay unmanaged")
	}
	if codex.NeedsUpdate {
		t.Fatal("expected unmanaged codex skill to skip update checks")
	}
	if codex.Version != "" {
		t.Fatalf("version = %q, want empty for unmanaged skill without marker", codex.Version)
	}
}

func TestDetectAgentsReportsExactLegacySkillAsManaged(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex", "skills", "futrixdata"), 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	path := filepath.Join(home, ".codex", "skills", "futrixdata", "SKILL.md")
	if err := os.WriteFile(path, mustReadLegacyFixture(t, "legacy_codex_v2.md"), 0o644); err != nil {
		t.Fatalf("write legacy skill: %v", err)
	}

	codex := findAgent(detectAgentsWithHome(home), AgentCodex)
	if !codex.Installed {
		t.Fatal("expected legacy codex skill to be detected as installed")
	}
	if !codex.Managed {
		t.Fatal("expected exact legacy codex skill to be treated as managed")
	}
	if !codex.NeedsUpdate {
		t.Fatal("expected exact legacy codex skill to require update")
	}
	if codex.Version != "legacy" {
		t.Fatalf("version = %q, want legacy", codex.Version)
	}
}

func TestDetectAgentsDoesNotAutoManageEditedLegacySkill(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex", "skills", "futrixdata"), 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	path := filepath.Join(home, ".codex", "skills", "futrixdata", "SKILL.md")
	edited := string(mustReadLegacyFixture(t, "legacy_codex_v2.md")) + "\n# user note\n"
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatalf("write edited legacy skill: %v", err)
	}

	codex := findAgent(detectAgentsWithHome(home), AgentCodex)
	if !codex.Installed {
		t.Fatal("expected edited legacy codex skill to stay installed")
	}
	if codex.Managed {
		t.Fatal("expected edited legacy codex skill to stay unmanaged")
	}
	if codex.NeedsUpdate {
		t.Fatal("expected edited legacy codex skill to skip update checks")
	}
	if codex.Version != "" {
		t.Fatalf("version = %q, want empty for edited legacy skill", codex.Version)
	}
}

func TestRefreshInstalledSkillsUpdatesManagedOutdatedFilesOnly(t *testing.T) {
	home := t.TempDir()

	claudeDir := filepath.Join(home, ".claude", "skills", "futrixdata")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir claude dir: %v", err)
	}
	claudePath := filepath.Join(claudeDir, "SKILL.md")
	outdated := strings.ReplaceAll(string(mustReadTemplate(t, AgentClaude)), "version="+SkillVersion, "version=0.9.0")
	if err := os.WriteFile(claudePath, []byte(outdated), 0o644); err != nil {
		t.Fatalf("write outdated skill: %v", err)
	}

	codexDir := filepath.Join(home, ".codex", "skills", "futrixdata")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	codexPath := filepath.Join(codexDir, "SKILL.md")
	if err := os.WriteFile(codexPath, []byte("# user managed\nversion=0.1.0\n"), 0o644); err != nil {
		t.Fatalf("write unmanaged skill: %v", err)
	}

	result := refreshInstalledSkillsWithHome(home, "")
	if len(result.Installed) != 1 {
		t.Fatalf("expected one managed outdated skill to be refreshed, got %#v", result.Installed)
	}
	gotClaude, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read updated claude skill: %v", err)
	}
	if !strings.Contains(string(gotClaude), "version="+SkillVersion) {
		t.Fatalf("expected claude skill to be updated to %q, got %q", SkillVersion, string(gotClaude))
	}
	gotCodex, err := os.ReadFile(codexPath)
	if err != nil {
		t.Fatalf("read codex skill: %v", err)
	}
	if string(gotCodex) != "# user managed\nversion=0.1.0\n" {
		t.Fatalf("expected unmanaged codex skill to stay untouched, got %q", string(gotCodex))
	}
}

func TestRefreshInstalledSkillsPreservesAccessKey(t *testing.T) {
	home := t.TempDir()

	claudeDir := filepath.Join(home, ".claude", "skills", "futrixdata")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir claude dir: %v", err)
	}
	claudePath := filepath.Join(claudeDir, "SKILL.md")

	rendered, err := renderSkillTemplate(AgentClaude, "agent_test_1234")
	if err != nil {
		t.Fatalf("render skill: %v", err)
	}
	outdated := strings.ReplaceAll(string(rendered), "version="+SkillVersion, "version=0.9.0")
	if err := os.WriteFile(claudePath, []byte(outdated), 0o644); err != nil {
		t.Fatalf("write outdated skill: %v", err)
	}

	result := refreshInstalledSkillsWithHome(home, "")
	if len(result.Installed) != 1 {
		t.Fatalf("expected one refreshed skill, got %#v", result.Installed)
	}

	updated, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read updated skill: %v", err)
	}
	content := string(updated)
	if !strings.Contains(content, "version="+SkillVersion) {
		t.Fatalf("expected updated version marker, got %q", content)
	}
	if !strings.Contains(content, "--agent-access-key agent_test_1234") {
		t.Fatalf("expected preserved access key, got %q", content)
	}
}

func TestRefreshInstalledSkillsAssignsAccessKeyForLegacyManagedSkill(t *testing.T) {
	home := t.TempDir()
	dataPath := filepath.Join(t.TempDir(), "datasources.json")

	codexDir := filepath.Join(home, ".codex", "skills", "futrixdata")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	codexPath := filepath.Join(codexDir, "SKILL.md")
	if err := os.WriteFile(codexPath, mustReadLegacyFixture(t, "legacy_codex_v2.md"), 0o644); err != nil {
		t.Fatalf("write legacy skill: %v", err)
	}

	result := refreshInstalledSkillsWithHome(home, dataPath)
	if len(result.Installed) != 1 || !result.Installed[0].Success {
		t.Fatalf("expected one successful refresh, got %#v", result.Installed)
	}

	updated, err := os.ReadFile(codexPath)
	if err != nil {
		t.Fatalf("read updated skill: %v", err)
	}
	content := string(updated)
	if !strings.Contains(content, "--agent-access-key agent_") {
		t.Fatalf("expected refreshed legacy skill to gain access key, got %q", content)
	}

	identities, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath)).ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(identities) != 1 {
		t.Fatalf("expected one detected identity, got %#v", identities)
	}
	if identities[0].AgentType != string(AgentCodex) {
		t.Fatalf("agent type = %q, want %q", identities[0].AgentType, AgentCodex)
	}
}

func TestRefreshInstalledSkillsBackfillsAccessKeyWithoutVersionBump(t *testing.T) {
	home := t.TempDir()
	dataPath := filepath.Join(t.TempDir(), "datasources.json")

	claudeDir := filepath.Join(home, ".claude", "skills", "futrixdata")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir claude dir: %v", err)
	}
	claudePath := filepath.Join(claudeDir, "SKILL.md")
	rendered, err := renderSkillTemplate(AgentClaude, "")
	if err != nil {
		t.Fatalf("render skill: %v", err)
	}
	if err := os.WriteFile(claudePath, rendered, 0o644); err != nil {
		t.Fatalf("write current version skill: %v", err)
	}

	result := refreshInstalledSkillsWithHome(home, dataPath)
	if len(result.Installed) != 1 || !result.Installed[0].Success {
		t.Fatalf("expected one successful refresh, got %#v", result.Installed)
	}

	updated, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read updated skill: %v", err)
	}
	content := string(updated)
	if !strings.Contains(content, "version="+SkillVersion) {
		t.Fatalf("expected version marker preserved, got %q", content)
	}
	if !strings.Contains(content, "--agent-access-key agent_") {
		t.Fatalf("expected current-version skill to gain access key, got %q", content)
	}

	identities, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath)).ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(identities) != 1 {
		t.Fatalf("expected one detected identity, got %#v", identities)
	}
	if identities[0].AgentType != string(AgentClaude) {
		t.Fatalf("agent type = %q, want %q", identities[0].AgentType, AgentClaude)
	}
}

func TestRefreshInstalledSkillsRecreatesIdentityForExistingKey(t *testing.T) {
	home := t.TempDir()
	dataPath := filepath.Join(t.TempDir(), "datasources.json")

	claudeDir := filepath.Join(home, ".claude", "skills", "futrixdata")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir claude dir: %v", err)
	}
	claudePath := filepath.Join(claudeDir, "SKILL.md")
	rendered, err := renderSkillTemplate(AgentClaude, "agent_test_1234")
	if err != nil {
		t.Fatalf("render skill: %v", err)
	}
	if err := os.WriteFile(claudePath, rendered, 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	result := refreshInstalledSkillsWithHome(home, dataPath)
	if len(result.Installed) != 0 {
		t.Fatalf("expected no rewrite for already-keyed current skill, got %#v", result.Installed)
	}

	identity, found, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath)).Get("agent_test_1234")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected identity to be recreated for existing key")
	}
	if identity.AgentType != string(AgentClaude) {
		t.Fatalf("agent type = %q, want %q", identity.AgentType, AgentClaude)
	}
}

func mustReadTemplate(t *testing.T, id AgentID) []byte {
	t.Helper()
	data, err := templates.ReadFile(templateFile(id))
	if err != nil {
		t.Fatalf("read template %s: %v", id, err)
	}
	return data
}

func mustReadLegacyFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read legacy fixture %s: %v", name, err)
	}
	return data
}

func findAgent(agents []Agent, id AgentID) Agent {
	for _, agent := range agents {
		if agent.ID == id {
			return agent
		}
	}
	return Agent{}
}

func TestSkillTemplatesMentionSensitivityWorkflow(t *testing.T) {
	for _, id := range AllAgentIDs() {
		data, err := templates.ReadFile(templateFile(id))
		if err != nil {
			t.Errorf("read template %s: %v", id, err)
			continue
		}
		content := string(data)
		if !strings.Contains(content, "get_sensitivity_config") {
			t.Errorf("%s template should mention get_sensitivity_config", id)
		}
		if !strings.Contains(content, "save_sensitivity_report") {
			t.Errorf("%s template should mention save_sensitivity_report", id)
		}
	}
}

func TestSkillTemplatesDoNotUseBareStdinExamples(t *testing.T) {
	for _, id := range AllAgentIDs() {
		data, err := templates.ReadFile(templateFile(id))
		if err != nil {
			t.Errorf("read template %s: %v", id, err)
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, "futrixdata-cli tool call") || !strings.Contains(line, "--stdin") {
				continue
			}
			if !strings.Contains(line, "| futrixdata-cli tool call") {
				t.Errorf("%s template contains bare --stdin example: %q", id, line)
			}
		}
	}
}

func TestSkillTemplatesDocumentDynamoDBPaginationSemantics(t *testing.T) {
	for _, id := range AllAgentIDs() {
		data, err := templates.ReadFile(templateFile(id))
		if err != nil {
			t.Errorf("read template %s: %v", id, err)
			continue
		}
		content := string(data)
		for _, want := range []string{
			"DynamoDB",
			"pageSize maps to evaluated items",
			"not guaranteed matched or returned rows",
			"empty or short page can still return nextToken",
			"Treat nextToken as opaque",
			"maxPages defaults to the risk policy cap",
			"strictLimits",
			"--stdin",
			"Do not hand-build opaque nextToken values in shell strings",
		} {
			if !strings.Contains(content, want) {
				t.Errorf("%s template missing DynamoDB pagination guidance %q", id, want)
			}
		}
	}
}

func TestSkillTemplatesDocumentIssue435ToolUsageGuidance(t *testing.T) {
	required := []string{
		"FUTRIXDATA_AGENT_ACCESS_KEY",
		"`--agent-access-key` takes precedence over `FUTRIXDATA_AGENT_ACCESS_KEY`",
		"FUTRIXDATA_AGENT_KEY",
		"### DynamoDB Bounded Pagination",
		"| `pageSize` | Service/tool hard limit | `<= 500`",
		"| `maxPages` | Risk-policy effective cap | `20`",
		"| `maxEvaluatedItems` | Risk-policy effective cap | `5000`",
		`"maxReturnedRows": 100`,
		`"maxPages": 20`,
		`"maxEvaluatedItems": 5000`,
		`spawnSync("futrixdata-cli"`,
		"payload.pagingToken = nextToken",
		"### Redis Dialect Quirks",
		"temporary limitation",
		"Do not present base64 encoding as the final solution",
		"### Approval Layers",
		"| Layer | What can stop the call | What to do |",
		"agent grants",
	}
	for _, id := range AllAgentIDs() {
		data, err := templates.ReadFile(templateFile(id))
		if err != nil {
			t.Errorf("read template %s: %v", id, err)
			continue
		}
		content := string(data)
		for _, want := range required {
			if !strings.Contains(content, want) {
				t.Errorf("%s template missing issue 435 guidance %q", id, want)
			}
		}
	}
}

func TestCLIInstallUninstall(t *testing.T) {
	dir := t.TempDir()
	orig := CLIInstallDir
	CLIInstallDir = dir
	defer func() { CLIInstallDir = orig }()

	dest := filepath.Join(dir, "futrixdata-cli")

	// Create a fake CLI binary.
	fakeBin := filepath.Join(t.TempDir(), "futrixdata-cli")
	os.WriteFile(fakeBin, []byte("#!/bin/sh\necho ok"), 0755)

	// Install.
	if err := InstallCLI(fakeBin); err != nil {
		t.Fatalf("InstallCLI: %v", err)
	}
	if !fileExists(dest) {
		t.Error("expected CLI binary at dest after install")
	}
	if target, err := os.Readlink(dest); err != nil {
		t.Errorf("expected symlink, got: %v", err)
	} else if target != fakeBin {
		t.Errorf("expected symlink to %s, got %s", fakeBin, target)
	}
	metaPath := cliInstallMetadataPath()
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read install metadata: %v", err)
	}
	var meta cliInstallMetadata
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatalf("unmarshal install metadata: %v", err)
	}
	if meta.SourcePath != fakeBin {
		t.Fatalf("sourcePath = %q, want %q", meta.SourcePath, fakeBin)
	}

	// Idempotent reinstall.
	if err := InstallCLI(fakeBin); err != nil {
		t.Fatalf("InstallCLI (idempotent): %v", err)
	}

	// Uninstall.
	if err := UninstallCLI(); err != nil {
		t.Fatalf("UninstallCLI: %v", err)
	}
	if fileExists(dest) {
		t.Error("expected CLI binary removed after uninstall")
	}
	if fileExists(metaPath) {
		t.Error("expected CLI install metadata removed after uninstall")
	}

	// Double uninstall is fine.
	if err := UninstallCLI(); err != nil {
		t.Fatalf("UninstallCLI (double): %v", err)
	}
}

func TestEnsureUnixShellPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}
	home := t.TempDir()
	dir := filepath.Join(home, ".local", "bin")

	// Create a .zshrc so the function has something to append to.
	zshrc := filepath.Join(home, ".zshrc")
	os.WriteFile(zshrc, []byte("# existing config\n"), 0644)

	// Temporarily override HOME so ensureUnixShellPath finds our test home.
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	if err := ensureUnixShellPath(dir); err != nil {
		t.Fatalf("ensureUnixShellPath: %v", err)
	}

	data, err := os.ReadFile(zshrc)
	if err != nil {
		t.Fatalf("read zshrc: %v", err)
	}
	if !strings.Contains(string(data), dir) {
		t.Error("expected .zshrc to contain CLI install dir")
	}

	// Idempotent: calling again should not duplicate the entry.
	if err := ensureUnixShellPath(dir); err != nil {
		t.Fatalf("ensureUnixShellPath (idempotent): %v", err)
	}
	data, _ = os.ReadFile(zshrc)
	if strings.Count(string(data), dir) != 1 {
		t.Error("expected exactly one PATH entry in .zshrc")
	}
}

func TestValidateCLIExecutableAgainstDesktopApp_AllowsAdjacentAppBinary(t *testing.T) {
	root := t.TempDir()
	cliPath := filepath.Join(root, cliBinName())
	appPath := filepath.Join(root, appBinNameForRuntime(runtime.GOOS))
	if err := os.WriteFile(cliPath, []byte("cli"), 0o755); err != nil {
		t.Fatalf("write cli: %v", err)
	}
	if err := os.WriteFile(appPath, []byte("app"), 0o755); err != nil {
		t.Fatalf("write app: %v", err)
	}

	if err := validateCLIExecutableAgainstDesktopApp(cliPath); err != nil {
		t.Fatalf("expected adjacent app to validate, got: %v", err)
	}
}

func TestValidateCLIExecutableAgainstDesktopApp_AllowsWithoutManagedAppMetadata(t *testing.T) {
	dir := t.TempDir()
	orig := CLIInstallDir
	CLIInstallDir = filepath.Join(t.TempDir(), "managed-cli")
	defer func() { CLIInstallDir = orig }()

	cliPath := filepath.Join(dir, cliBinName())
	if err := os.WriteFile(cliPath, []byte("cli"), 0o755); err != nil {
		t.Fatalf("write cli: %v", err)
	}

	if err := validateCLIExecutableAgainstDesktopApp(cliPath); err != nil {
		t.Fatalf("expected unmanaged cli to validate, got: %v", err)
	}
}

func TestValidateCLIExecutableAgainstDesktopApp_IgnoresManagedMetadataForStandaloneCLI(t *testing.T) {
	dir := t.TempDir()
	orig := CLIInstallDir
	CLIInstallDir = filepath.Join(t.TempDir(), "managed-cli")
	defer func() { CLIInstallDir = orig }()

	cliPath := filepath.Join(dir, cliBinName())
	if err := os.WriteFile(cliPath, []byte("cli"), 0o755); err != nil {
		t.Fatalf("write cli: %v", err)
	}
	if err := os.MkdirAll(CLIInstallDir, 0o755); err != nil {
		t.Fatalf("mkdir managed install dir: %v", err)
	}
	if err := writeCLIInstallMetadata(cliInstallMetadata{
		SourcePath: filepath.Join(CLIInstallDir, cliBinName()),
		AppPath:    filepath.Join(dir, "missing", "FutrixData.app"),
	}); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	if err := validateCLIExecutableAgainstDesktopApp(cliPath); err != nil {
		t.Fatalf("expected standalone cli to ignore managed metadata, got: %v", err)
	}
}

func TestValidateCLIExecutableAgainstDesktopApp_FailsWhenRecordedAppMissing(t *testing.T) {
	dir := t.TempDir()
	orig := CLIInstallDir
	CLIInstallDir = dir
	defer func() { CLIInstallDir = orig }()

	cliPath := filepath.Join(dir, cliBinName())
	if err := os.WriteFile(cliPath, []byte("cli"), 0o755); err != nil {
		t.Fatalf("write cli: %v", err)
	}
	if err := writeCLIInstallMetadata(cliInstallMetadata{
		SourcePath: filepath.Join(dir, "FutrixData.app", "Contents", "MacOS", cliBinName()),
		AppPath:    filepath.Join(dir, "FutrixData.app"),
	}); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	err := validateCLIExecutableAgainstDesktopApp(cliPath)
	if err == nil {
		t.Fatal("expected validation failure when recorded app is missing")
	}
	if !strings.Contains(err.Error(), "https://futrixdata.com/") {
		t.Fatalf("expected install url in error, got: %v", err)
	}
}

func TestValidateCLIExecutableAgainstDesktopApp_FailsForManagedInstallWithoutMetadata(t *testing.T) {
	dir := t.TempDir()
	orig := CLIInstallDir
	CLIInstallDir = dir
	defer func() { CLIInstallDir = orig }()

	cliPath := filepath.Join(dir, cliBinName())
	if err := os.WriteFile(cliPath, []byte("cli"), 0o755); err != nil {
		t.Fatalf("write cli: %v", err)
	}

	err := validateCLIExecutableAgainstDesktopApp(cliPath)
	if err == nil {
		t.Fatal("expected managed cli without metadata to fail validation")
	}
	if !strings.Contains(err.Error(), "https://futrixdata.com/") {
		t.Fatalf("expected install url in error, got: %v", err)
	}
}

func TestValidateCLIExecutableAgainstDesktopApp_FailsForManagedInstallWithoutAppPath(t *testing.T) {
	dir := t.TempDir()
	orig := CLIInstallDir
	CLIInstallDir = dir
	defer func() { CLIInstallDir = orig }()

	cliPath := filepath.Join(dir, cliBinName())
	if err := os.WriteFile(cliPath, []byte("cli"), 0o755); err != nil {
		t.Fatalf("write cli: %v", err)
	}
	if err := writeCLIInstallMetadata(cliInstallMetadata{
		SourcePath: cliPath,
	}); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	err := validateCLIExecutableAgainstDesktopApp(cliPath)
	if err == nil {
		t.Fatal("expected managed cli without app path to fail validation")
	}
	if !strings.Contains(err.Error(), "https://futrixdata.com/") {
		t.Fatalf("expected install url in error, got: %v", err)
	}
}
