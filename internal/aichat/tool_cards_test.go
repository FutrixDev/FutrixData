package aichat

import (
	"strings"
	"testing"
)

func TestToolCards_ContainsPurposeUseWhenAvoidWhen(t *testing.T) {
	cards := defaultToolCards()
	if len(cards) == 0 {
		t.Fatalf("expected default tool cards")
	}

	card, ok := findToolCard(cards, "search_knowledge")
	if !ok {
		t.Fatalf("expected search_knowledge tool card")
	}
	if strings.TrimSpace(card.Purpose) == "" {
		t.Fatalf("expected purpose to be populated")
	}
	if len(card.UseWhen) == 0 {
		t.Fatalf("expected useWhen guidance")
	}
	if len(card.AvoidWhen) == 0 {
		t.Fatalf("expected avoidWhen guidance")
	}
	if len(card.Inputs) == 0 {
		t.Fatalf("expected inputs guidance")
	}
	if len(card.FollowUp) == 0 {
		t.Fatalf("expected followUp guidance")
	}
	if len(card.CostPolicy) == 0 {
		t.Fatalf("expected cost policy guidance")
	}
}

func TestToolCards_RanksSearchKnowledgeDescribeExecuteFirstOnConsole(t *testing.T) {
	req := TurnRequest{
		Messages: []Message{
			{
				Role: "user",
				Content: `分析为什么 SELECT * FROM "xxx" WHERE "uid" = 'yyy' AND "aid" = 'vvv' 能查到，` +
					`但 SELECT * FROM "xxx" WHERE "aid" = 'vvv' 查不到`,
			},
		},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
			CurrentEntity:         "xxx",
		},
	}

	cards := rankToolCards(buildToolCardContext(req, nil))
	if len(cards) < 3 {
		t.Fatalf("expected at least 3 ranked cards, got %d", len(cards))
	}

	got := []string{cards[0].Name, cards[1].Name, cards[2].Name}
	want := []string{"search_knowledge", "describe_entity", "execute_statement"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected top tool cards %v, got %v", want, got)
		}
	}
}

func TestSearchKnowledge_ToolCardDescribesFocusHintAndProgressiveExpansion(t *testing.T) {
	cards := defaultToolCards()
	card, ok := findToolCard(cards, "search_knowledge")
	if !ok {
		t.Fatalf("expected search_knowledge tool card")
	}
	joined := strings.Join(card.Inputs, " ")
	if !strings.Contains(joined, "focus") || !strings.Contains(joined, "working context") || !strings.Contains(joined, "type") || !strings.Contains(joined, "all") {
		t.Fatalf("expected search_knowledge card to describe focus hint + working context expansion, got %q", joined)
	}
}

func TestToolCards_ContainsMemorySavePatternGuidance(t *testing.T) {
	cards := defaultToolCards()
	card, ok := findToolCard(cards, "memory_save")
	if !ok {
		t.Fatalf("expected memory_save tool card")
	}
	if !strings.Contains(strings.Join(card.UseWhen, " "), "reusable") {
		t.Fatalf("expected memory_save to describe reusable pattern use, got %+v", card.UseWhen)
	}
	if !strings.Contains(strings.Join(card.AvoidWhen, " "), "event") {
		t.Fatalf("expected memory_save to avoid raw event logging, got %+v", card.AvoidWhen)
	}
}

func TestSystemPrompt_DoesNotHardcodeExecuteFirstOrSchemaFirst(t *testing.T) {
	prompt := defaultBaseSystemPrompt
	if strings.Contains(prompt, "Treat “查询/查看/列出/获取/前 N 条” as execute intent by default") {
		t.Fatalf("expected prompt to avoid hardcoded execute-first rule")
	}
	if strings.Contains(prompt, "prefer schema/index inspection first") {
		t.Fatalf("expected prompt to avoid hardcoded schema-first rule")
	}
	if !strings.Contains(prompt, "minimal sufficient evidence source") {
		t.Fatalf("expected prompt to emphasize minimal sufficient evidence source")
	}
	if !strings.Contains(prompt, "Tools are peer evidence sources") {
		t.Fatalf("expected prompt to describe tools as peer evidence sources")
	}
	if !strings.Contains(prompt, "focus hint") {
		t.Fatalf("expected prompt to describe page context as focus hint")
	}
	if !strings.Contains(prompt, "working context") {
		t.Fatalf("expected prompt to describe a working context")
	}
	if !strings.Contains(prompt, `intent.currentFocus="prefer_current"`) {
		t.Fatalf("expected prompt to describe structured focus intent")
	}
	if !strings.Contains(prompt, "deep exploration successfully resolves a focus mismatch") {
		t.Fatalf("expected prompt to describe memory closure after successful focus mismatch exploration")
	}
}

func TestDatasourceRoutingHint_DynamoDBUsesSearchKnowledgeForConstraints(t *testing.T) {
	hint := normalizePromptModule(dynamodbPromptModule)
	if !strings.Contains(hint, "search_knowledge") {
		t.Fatalf("expected dynamodb routing hint to mention search_knowledge")
	}
	if strings.Contains(hint, "prefer schema/index inspection first") {
		t.Fatalf("expected dynamodb routing hint to avoid fixed schema-first wording")
	}
	if !strings.Contains(hint, "Partition Key") {
		t.Fatalf("expected dynamodb routing hint to mention live key facts")
	}
	if !strings.Contains(hint, "Dialect is partiql") {
		t.Fatalf("expected dynamodb routing hint to identify PartiQL dialect")
	}
	if !strings.Contains(hint, "MySQL hints") {
		t.Fatalf("expected dynamodb routing hint to warn against MySQL syntax")
	}
}

func TestBuildToolsSection_GroupsToolsByCapabilityInsteadOfRanking(t *testing.T) {
	section := buildToolsSection(defaultToolCards())
	if !strings.Contains(section, "Discovery tools:") {
		t.Fatalf("expected discovery tools group, got: %s", section)
	}
	if !strings.Contains(section, "Action tools:") {
		t.Fatalf("expected action tools group, got: %s", section)
	}
	if !strings.Contains(section, "Memory tools:") {
		t.Fatalf("expected memory tools group, got: %s", section)
	}
	if !strings.Contains(section, "- web_search") {
		t.Fatalf("expected grouped tool section to keep web_search visible, got: %s", section)
	}
	if strings.Contains(section, "ranked for the current turn") {
		t.Fatalf("expected tools section to avoid ranked wording, got: %s", section)
	}
	if strings.Contains(section, "1. search_knowledge") {
		t.Fatalf("expected tools section to avoid numeric ranking, got: %s", section)
	}
}
