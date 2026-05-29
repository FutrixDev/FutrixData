package aichat

import (
	"fmt"
	"sort"
	"strings"
)

type ToolCard struct {
	Name       string
	Purpose    string
	UseWhen    []string
	AvoidWhen  []string
	Inputs     []string
	Examples   []string
	FollowUp   []string
	CostPolicy []string
}

type toolCardContext struct {
	RouteName       string
	DatasourceID    string
	DatasourceType  string
	UserText        string
	RecentToolNames []string
}

func defaultToolCards() []ToolCard {
	return []ToolCard{
		{
			Name:    "search_knowledge",
			Purpose: "Fetch the smallest useful static knowledge snippet for datasource constraints, business conventions, field mappings, or runbook guidance.",
			UseWhen: []string{
				"The question depends on datasource-specific constraints or business knowledge that may already be documented.",
				"You need syntax, quoting, predicate/index rules, field naming conventions, or troubleshooting guidance.",
			},
			AvoidWhen: []string{
				"The current page context or recent tool results already answer the question.",
				"You need live schema facts or runtime verification instead of static guidance.",
			},
			Inputs: []string{
				"Use query to describe the missing fact, not the whole user question.",
				"If a full query is awkward, you may use topic/intent/hint to trigger datasource-aware query templates.",
				"Treat page context as a focus hint; prefer the active working context when one has already been discovered.",
				"Expand progressively from the narrowest plausible scope: current/working target, then type, then all.",
			},
			Examples: []string{
				`search_knowledge {query:"dynamodb predicate/index rule for filtering by non-key attribute", scope:"current"}`,
			},
			FollowUp: []string{
				"If the retrieved guidance resolves the ambiguity, answer directly.",
				"If knowledge is insufficient, move to describe_entity or execute_statement based on the remaining gap.",
			},
			CostPolicy: []string{
				"Cheap, no approval.",
				"Prefer this before execution when static guidance is enough.",
			},
		},
		{
			Name:    "memory_save",
			Purpose: "Persist one reusable troubleshooting or tool-selection pattern into long-term memory.",
			UseWhen: []string{
				"The current turn discovered a reusable pattern that can help future threads.",
				"A wrong or low-value path was corrected and the corrected path is generalizable.",
			},
			AvoidWhen: []string{
				"You only have a raw event log, one-off SQL text, ids, or other case-specific details.",
				"The conclusion is still speculative or not reusable.",
			},
			Inputs: []string{
				"Describe the pattern with problem, signals, avoid, do, and why.",
				"Keep it abstract and reusable; do not copy raw event payloads.",
			},
			Examples: []string{
				`memory_save {problem:"avoid duplicate execute loops", signals:["same statement repeats"], avoid:["re-run execute_statement"], do:["reuse existing evidence"], why:"Repeated execution adds cost without reducing ambiguity.", confidence:0.9}`,
			},
			FollowUp: []string{
				"Save at most once per turn, then continue or finalize normally.",
			},
			CostPolicy: []string{
				"Cheap, no approval.",
				"Use sparingly for reusable patterns only.",
			},
		},
		{
			Name:    "describe_entity",
			Purpose: "Fetch live schema and entity metadata such as fields, keys, indexes, and entity shape.",
			UseWhen: []string{
				"You need exact field names, keys, indexes, or entity structure.",
				"The user asks why one predicate works and another does not, and live entity facts may explain it.",
			},
			AvoidWhen: []string{
				"You only need static vendor or business guidance that search_knowledge can answer.",
				"The answer already exists in recent tool results.",
			},
			Inputs: []string{
				"Use datasourceId from the established working context when available; otherwise fall back to the page focus only as a hint.",
				"Use the discovered or referenced entity name instead of assuming the current page entity is the target.",
			},
			Examples: []string{
				`describe_entity {datasourceId:"ds_xxx", name:"orders", database:"appdb"}`,
			},
			FollowUp: []string{
				"Explain the issue directly if the schema facts are sufficient.",
				"Use execute_statement only if a live verification gap remains.",
			},
			CostPolicy: []string{
				"Cheap, no approval.",
			},
		},
		{
			Name:    "execute_statement",
			Purpose: "Run a datasource statement to verify a remaining ambiguity or fulfill an explicit execution request.",
			UseWhen: []string{
				"The user explicitly asks to run/query data.",
				"Knowledge and schema facts are still insufficient, and runtime verification is necessary.",
			},
			AvoidWhen: []string{
				"The answer can already be given from page context, recent results, search_knowledge, or describe_entity.",
				"You would only be repeating the same execution without adding evidence.",
			},
			Inputs: []string{
				"Use raw datasource-appropriate statement text.",
				"Resolve datasourceId/database from the working context first; only fall back to page focus when no better target exists.",
				"Keep pageSize small and use key/index-friendly predicates when possible.",
			},
			Examples: []string{
				`execute_statement {datasourceId:"ds_xxx", database:"appdb", statement:"SELECT * FROM orders WHERE id = 1", pageSize:20}`,
			},
			FollowUp: []string{
				"Use the result summary to answer directly or decide whether analyze_result is needed.",
			},
			CostPolicy: []string{
				"May require approval.",
				"Use only when runtime evidence is necessary.",
			},
		},
		{
			Name:    "analyze_result",
			Purpose: "Analyze the previous AI console result sample.",
			UseWhen: []string{
				"The user asks to summarize or analyze the last result already shown in the app.",
			},
			AvoidWhen: []string{
				"No recent AI console result exists.",
			},
			Inputs: []string{
				"Optional question narrows the analysis goal.",
			},
			FollowUp: []string{
				"Return a concise human explanation after approval.",
			},
			CostPolicy: []string{
				"Requires approval because result samples are shared with the model.",
			},
		},
		{
			Name:    "create_visualization",
			Purpose: "Generate a visualization spec from the previous AI console result sample.",
			UseWhen: []string{
				"The user asks to visualize the last result.",
			},
			AvoidWhen: []string{
				"No recent AI console result exists.",
			},
			Inputs: []string{
				"Optional question can specify chart intent.",
			},
			FollowUp: []string{
				"Navigate to visualization after approval.",
			},
			CostPolicy: []string{
				"Requires approval because result samples are shared with the model.",
			},
		},
		{
			Name:       "list_datasources",
			Purpose:    "List available datasources.",
			UseWhen:    []string{"The user asks what datasources are available.", "You need to discover whether the target object may live outside the current focus datasource."},
			AvoidWhen:  []string{"A stable working target is already known and datasource discovery is not needed."},
			Inputs:     []string{"No arguments."},
			FollowUp:   []string{"Use get_datasource or navigate_to_datasource next if needed."},
			CostPolicy: []string{"Cheap, no approval."},
		},
		{
			Name:       "get_datasource",
			Purpose:    "Fetch datasource metadata for a known datasource.",
			UseWhen:    []string{"You need details about one datasource."},
			AvoidWhen:  []string{"The datasource metadata already exists in context."},
			Inputs:     []string{"Provide id."},
			FollowUp:   []string{"Use the datasource details to guide the next tool call."},
			CostPolicy: []string{"Cheap, no approval."},
		},
		{
			Name:       "list_databases",
			Purpose:    "List databases within a datasource.",
			UseWhen:    []string{"The user asks to discover databases or you need database names."},
			AvoidWhen:  []string{"The database is already known."},
			Inputs:     []string{"Provide datasourceId and optional pattern."},
			FollowUp:   []string{"Use list_entities or describe_entity next if needed."},
			CostPolicy: []string{"Cheap, no approval."},
		},
		{
			Name:       "list_entities",
			Purpose:    "List entities/tables/collections within a datasource or database.",
			UseWhen:    []string{"You need to discover possible entities.", "The user names an object like a table/collection/index but the current focus does not clearly contain it."},
			AvoidWhen:  []string{"The entity is already known."},
			Inputs:     []string{"Provide datasourceId, optional database, optional pattern."},
			FollowUp:   []string{"Use describe_entity for the chosen entity."},
			CostPolicy: []string{"Cheap, no approval."},
		},
		{
			Name:       "explain_statement",
			Purpose:    "Inspect statement execution plan when the datasource supports explain.",
			UseWhen:    []string{"You specifically need an explain plan."},
			AvoidWhen:  []string{"The datasource does not support explain or the question is conceptual."},
			Inputs:     []string{"Provide statement and datasourceId."},
			FollowUp:   []string{"Use the explain summary to reason about index use or scan risk."},
			CostPolicy: []string{"Cheap, no approval."},
		},
		{
			Name:       "web_search",
			Purpose:    "Retrieve latest public web information.",
			UseWhen:    []string{"The question depends on external up-to-date public information."},
			AvoidWhen:  []string{"The answer depends only on local app context or built-in knowledge."},
			Inputs:     []string{"Provide a narrow public query."},
			FollowUp:   []string{"Answer with cited links."},
			CostPolicy: []string{"Network call, no approval."},
		},
		{
			Name:       "get_redis_command_docs",
			Purpose:    "Fetch Redis command reference guidance.",
			UseWhen:    []string{"You need Redis command syntax or behavior details."},
			AvoidWhen:  []string{"A different datasource is active or the command is already known from search_knowledge."},
			Inputs:     []string{"Provide datasourceId and command."},
			FollowUp:   []string{"Apply the command docs to build the next answer or statement."},
			CostPolicy: []string{"Cheap, no approval."},
		},
		{
			Name:       "get_schema_knowledge",
			Purpose:    "Fetch schema-oriented knowledge derived from customer knowledge sources.",
			UseWhen:    []string{"You need higher-level schema/business context for a datasource entity."},
			AvoidWhen:  []string{"Live schema facts from describe_entity are enough."},
			Inputs:     []string{"Provide datasourceId and optional entity/database."},
			FollowUp:   []string{"Answer directly or refine with describe_entity."},
			CostPolicy: []string{"Cheap, no approval."},
		},
		{
			Name:       "get_er_knowledge",
			Purpose:    "Fetch ER-style knowledge for a datasource/database.",
			UseWhen:    []string{"The user asks about relationships across entities."},
			AvoidWhen:  []string{"A single entity answer is sufficient."},
			Inputs:     []string{"Provide datasourceId and optional database."},
			FollowUp:   []string{"Use it to reason about joins/relationships."},
			CostPolicy: []string{"Cheap, no approval."},
		},
		{
			Name:       "create_datasource",
			Purpose:    "Create a datasource connection.",
			UseWhen:    []string{"The user wants to add a datasource."},
			AvoidWhen:  []string{"The user only asks for instructions or configuration advice."},
			Inputs:     []string{"Collect connection parameters explicitly."},
			FollowUp:   []string{"Wait for approval and then verify creation."},
			CostPolicy: []string{"Requires approval."},
		},
		{
			Name:       "delete_datasource",
			Purpose:    "Delete a datasource connection.",
			UseWhen:    []string{"The user explicitly wants to delete a datasource."},
			AvoidWhen:  []string{"The user has not clearly confirmed deletion."},
			Inputs:     []string{"Use datasourceId when possible."},
			FollowUp:   []string{"Wait for approval and then confirm removal."},
			CostPolicy: []string{"Requires approval."},
		},
		{
			Name:       "navigate_to_datasource",
			Purpose:    "Navigate the UI to a datasource-related page.",
			UseWhen:    []string{"The user asks to open or switch to a datasource page."},
			AvoidWhen:  []string{"The answer can stay in chat without UI navigation."},
			Inputs:     []string{"Use datasourceId or name, plus optional target."},
			FollowUp:   []string{"Continue the conversation on the destination page if needed."},
			CostPolicy: []string{"Cheap, no approval."},
		},
	}
}

func findToolCard(cards []ToolCard, name string) (ToolCard, bool) {
	needle := strings.TrimSpace(name)
	for i := range cards {
		if cards[i].Name == needle {
			return cards[i], true
		}
	}
	return ToolCard{}, false
}

func buildToolCardContext(req TurnRequest, recentToolNames []string) toolCardContext {
	return toolCardContext{
		RouteName:       strings.ToLower(strings.TrimSpace(req.PageContext.RouteName)),
		DatasourceID:    strings.TrimSpace(req.PageContext.CurrentDatasourceID),
		DatasourceType:  strings.ToLower(strings.TrimSpace(req.PageContext.CurrentDatasourceType)),
		UserText:        strings.ToLower(strings.TrimSpace(lastToolCardUserText(req.Messages))),
		RecentToolNames: append([]string(nil), recentToolNames...),
	}
}

func rankToolCards(ctx toolCardContext) []ToolCard {
	cards := defaultToolCards()
	type rankedToolCard struct {
		card  ToolCard
		score int
	}
	ranked := make([]rankedToolCard, 0, len(cards))
	for i := range cards {
		score := scoreToolCard(cards[i], ctx)
		ranked = append(ranked, rankedToolCard{card: cards[i], score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].card.Name < ranked[j].card.Name
		}
		return ranked[i].score > ranked[j].score
	})
	out := make([]ToolCard, 0, len(ranked))
	for i := range ranked {
		out = append(out, ranked[i].card)
	}
	return out
}

func scoreToolCard(card ToolCard, ctx toolCardContext) int {
	score := 0
	if ctx.RouteName == "console" {
		switch card.Name {
		case "search_knowledge":
			score += 80
		case "describe_entity":
			score += 75
		case "execute_statement":
			score += 70
		case "analyze_result", "create_visualization":
			score += 40
		}
	}

	if ctx.DatasourceID != "" {
		switch card.Name {
		case "search_knowledge", "describe_entity", "execute_statement", "get_schema_knowledge", "get_er_knowledge":
			score += 15
		}
	}

	if ctx.DatasourceType == "dynamodb" {
		switch card.Name {
		case "search_knowledge":
			score += 35
		case "describe_entity":
			score += 30
		case "execute_statement":
			score += 25
		}
	}

	if looksLikeDiagnosticQuestion(ctx.UserText) {
		switch card.Name {
		case "search_knowledge":
			score += 40
		case "describe_entity":
			score += 35
		case "execute_statement":
			score += 10
		}
	}

	if strings.Contains(ctx.UserText, "visual") || strings.Contains(ctx.UserText, "图") {
		if card.Name == "create_visualization" {
			score += 60
		}
	}
	if strings.Contains(ctx.UserText, "analy") || strings.Contains(ctx.UserText, "总结") || strings.Contains(ctx.UserText, "分析结果") {
		if card.Name == "analyze_result" {
			score += 60
		}
	}
	if asksToRunQuery(ctx.UserText) && card.Name == "execute_statement" {
		score += 20
	}
	if strings.Contains(ctx.UserText, "remember") || strings.Contains(ctx.UserText, "记住") || strings.Contains(ctx.UserText, "memory") || strings.Contains(ctx.UserText, "模式") {
		if card.Name == "memory_save" {
			score += 35
		}
	}

	for _, recent := range ctx.RecentToolNames {
		if strings.TrimSpace(recent) == card.Name {
			score -= 5
		}
	}
	return score
}

func looksLikeDiagnosticQuestion(text string) bool {
	if text == "" {
		return false
	}
	keywords := []string{
		"why", "reason", "原因", "为什么", "查不到", "搜不到", "works but", "不能", "diagnos", "difference", "区别",
	}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func asksToRunQuery(text string) bool {
	if text == "" {
		return false
	}
	keywords := []string{
		"run", "execute", "查询", "执行", "select ", "limit ", "查一下",
	}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func lastToolCardUserText(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") {
			return messages[i].Content
		}
	}
	return ""
}

func buildToolsSection(cards []ToolCard) string {
	if len(cards) == 0 {
		cards = defaultToolCards()
	}
	groups := []struct {
		title string
		names map[string]struct{}
	}{
		{title: "Discovery tools", names: toolNameSet("search_knowledge", "list_datasources", "get_datasource", "list_databases", "list_entities", "describe_entity", "get_schema_knowledge", "get_er_knowledge", "get_redis_command_docs", "web_search")},
		{title: "Action tools", names: toolNameSet("explain_statement", "execute_statement", "analyze_result", "create_visualization")},
		{title: "Navigation tools", names: toolNameSet("navigate_to_datasource")},
		{title: "Memory tools", names: toolNameSet("memory_save")},
		{title: "Admin tools", names: toolNameSet("create_datasource", "delete_datasource")},
	}
	var b strings.Builder
	seen := make(map[string]struct{}, len(cards))
	b.WriteString("Available tools grouped by capability:\n")
	for _, group := range groups {
		groupCards := cardsInGroup(cards, group.names)
		if len(groupCards) == 0 {
			continue
		}
		b.WriteString(group.title + ":\n")
		for _, card := range groupCards {
			seen[card.Name] = struct{}{}
			b.WriteString(fmt.Sprintf("- %s\n", card.Name))
			b.WriteString(fmt.Sprintf("  purpose: %s\n", card.Purpose))
			if len(card.UseWhen) > 0 {
				b.WriteString(fmt.Sprintf("  use_when: %s\n", strings.Join(card.UseWhen, " | ")))
			}
			if len(card.AvoidWhen) > 0 {
				b.WriteString(fmt.Sprintf("  avoid_when: %s\n", strings.Join(card.AvoidWhen, " | ")))
			}
			if len(card.Inputs) > 0 {
				b.WriteString(fmt.Sprintf("  inputs: %s\n", strings.Join(card.Inputs, " | ")))
			}
			if len(card.FollowUp) > 0 {
				b.WriteString(fmt.Sprintf("  follow_up: %s\n", strings.Join(card.FollowUp, " | ")))
			}
			if len(card.CostPolicy) > 0 {
				b.WriteString(fmt.Sprintf("  cost_policy: %s\n", strings.Join(card.CostPolicy, " | ")))
			}
		}
	}
	if extras := uncategorizedCards(cards, seen); len(extras) > 0 {
		b.WriteString("Other tools:\n")
		for _, card := range extras {
			b.WriteString(fmt.Sprintf("- %s\n", card.Name))
			b.WriteString(fmt.Sprintf("  purpose: %s\n", card.Purpose))
			if len(card.UseWhen) > 0 {
				b.WriteString(fmt.Sprintf("  use_when: %s\n", strings.Join(card.UseWhen, " | ")))
			}
			if len(card.AvoidWhen) > 0 {
				b.WriteString(fmt.Sprintf("  avoid_when: %s\n", strings.Join(card.AvoidWhen, " | ")))
			}
			if len(card.Inputs) > 0 {
				b.WriteString(fmt.Sprintf("  inputs: %s\n", strings.Join(card.Inputs, " | ")))
			}
			if len(card.FollowUp) > 0 {
				b.WriteString(fmt.Sprintf("  follow_up: %s\n", strings.Join(card.FollowUp, " | ")))
			}
			if len(card.CostPolicy) > 0 {
				b.WriteString(fmt.Sprintf("  cost_policy: %s\n", strings.Join(card.CostPolicy, " | ")))
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func toolNameSet(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[strings.TrimSpace(name)] = struct{}{}
	}
	return out
}

func cardsInGroup(cards []ToolCard, names map[string]struct{}) []ToolCard {
	out := make([]ToolCard, 0, len(cards))
	for _, card := range cards {
		if _, ok := names[card.Name]; ok {
			out = append(out, card)
		}
	}
	return out
}

func uncategorizedCards(cards []ToolCard, seen map[string]struct{}) []ToolCard {
	out := make([]ToolCard, 0, len(cards))
	for _, card := range cards {
		if _, ok := seen[card.Name]; ok {
			continue
		}
		out = append(out, card)
	}
	return out
}
