package aichat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func defaultCreateVisualizationAssistantMessage(locale uiLocale, stored storedAnalysisResult) string {
	rows := len(stored.Rows)
	if locale == uiLocaleZH {
		if stored.RowsTruncated {
			return fmt.Sprintf("我可以把刚才的查询结果生成可视化（将发送前 %d 行给 AI 模型生成图表配置，需要你先在审批卡里确认）。", rows)
		}
		return fmt.Sprintf("我可以把刚才的查询结果生成可视化（将发送 %d 行给 AI 模型生成图表配置，需要你先在审批卡里确认）。", rows)
	}
	if stored.RowsTruncated {
		return fmt.Sprintf("I can generate a visualization from the last query result (will send the first %d rows to the AI model to generate a chart spec; approval required).", rows)
	}
	return fmt.Sprintf("I can generate a visualization from the last query result (will send %d rows to the AI model to generate a chart spec; approval required).", rows)
}

func summarizeCreateVisualization(locale uiLocale, stored storedAnalysisResult) string {
	rows := len(stored.Rows)
	if locale == uiLocaleZH {
		if stored.RowsTruncated {
			return fmt.Sprintf("将上次结果前 %d 行发送给 AI 生成可视化（已截断）", rows)
		}
		return fmt.Sprintf("将上次结果 %d 行发送给 AI 生成可视化", rows)
	}
	if stored.RowsTruncated {
		return fmt.Sprintf("Send first %d rows to AI to generate visualization (truncated)", rows)
	}
	return fmt.Sprintf("Send %d rows to AI to generate visualization", rows)
}

func sanitizeCreateVisualizationPayload(stored storedAnalysisResult) map[string]any {
	return sanitizeAnalyzeResultPayload(stored)
}

func visualizationSystemPrompt(locale uiLocale) string {
	if locale == uiLocaleZH {
		return strings.TrimSpace(`
你是 FutrixData 的“可视化生成助手”。

用户已经明确批准把“上一次 AI 查询结果的部分行数据”发送给你，用于生成可视化配置。

输出要求（严格）：
- 只输出一个 JSON 对象（不要输出 Markdown，不要输出 code fence）
- JSON 结构必须是：
  {"title":"...","renderer":"echarts|three|vega_lite","spec":{...}}
- renderer 只能是 "echarts" / "three" / "vega_lite"

ECharts 规范（renderer="echarts"）：
- spec 必须是 ECharts option（纯 JSON）
- 优先使用 dataset: { source: rows } + encode 映射
- 不要输出任何 JS 函数（formatter 等只能用字符串模板）

Vega-Lite 规范（renderer="vega_lite"）：
- spec 必须是 Vega-Lite v5 JSON（纯 JSON）
- spec 应包含 $schema: https://vega.github.io/schema/vega-lite/v5.json
- 优先使用 data: { values: rows }（使用已提供样本行）
- encoding 的 field 必须来自 rows 里真实存在的列名
- 不要输出任何 JS 函数

Three.js 规范（renderer="three"）：
- spec 必须是纯 JSON，且遵循以下结构之一：
  1) 3D 散点图：
     {
       "type":"scatter3d",
       "points":[{"x":1,"y":2,"z":3,"color":"#4f46e5","size":1,"label":"..."}],
       "axes":{"x":"...","y":"...","z":"..."},
       "background":"#0b1020"
     }

选择规则：
- 默认使用 Vega-Lite；只有当用户明确要 3D / three.js，或数据天然是 3 个数值维度时，才用 three.js
- 只有在 Vega-Lite 很难表达目标图表时，才回退到 ECharts
- 如字段/类型不清晰，选择最稳妥的图表（例如：时间序列→折线；类目对比→柱状；分布→直方/箱线）

安全：
- 不要包含任何密钥/密码/token
`)
	}

	return strings.TrimSpace(`
You are FutrixData's visualization generator.

The user has explicitly approved sending (sampled) rows from the most recent AI query result to generate a visualization.

Output requirements (strict):
- Output EXACTLY ONE JSON object (no Markdown, no code fences)
- JSON shape must be:
  {"title":"...","renderer":"echarts|three|vega_lite","spec":{...}}
- renderer must be either "echarts", "three", or "vega_lite"

ECharts rules (renderer="echarts"):
- spec must be a pure-JSON ECharts option
- Prefer dataset: { source: rows } + encode mapping
- Do NOT output any JS functions (formatter etc must be string templates only)

Vega-Lite rules (renderer="vega_lite"):
- spec must be a pure-JSON Vega-Lite v5 spec
- spec should include $schema: https://vega.github.io/schema/vega-lite/v5.json
- Prefer data: { values: rows } using the provided sample rows
- Every encoding field must exist in the provided rows columns
- Do NOT output any JS functions

Three.js rules (renderer="three"):
- spec must be pure JSON and follow one of the supported schemas:
  1) 3D scatter:
     {
       "type":"scatter3d",
       "points":[{"x":1,"y":2,"z":3,"color":"#4f46e5","size":1,"label":"..."}],
       "axes":{"x":"...","y":"...","z":"..."},
       "background":"#0b1020"
     }

Selection:
- Default to Vega-Lite; use three.js only when the user explicitly wants 3D/three.js or data naturally has 3 numeric dimensions.
- Use ECharts only when Vega-Lite is not a good fit for the goal.

Safety:
- Never include secrets (API keys, passwords, tokens).
`)
}

func visualizationUserContent(locale uiLocale, stored storedAnalysisResult, question string) string {
	rowsJSON, err := json.Marshal(stored.Rows)
	if err != nil {
		rowsJSON = []byte("[]")
	}

	q := strings.TrimSpace(question)
	if q == "" {
		if locale == uiLocaleZH {
			q = "请基于这些结果生成一个合适的可视化。"
		} else {
			q = "Please generate a suitable visualization based on these results."
		}
	}

	var b strings.Builder
	if locale == uiLocaleZH {
		b.WriteString("用户可视化需求：\n")
		b.WriteString(q + "\n\n")
		b.WriteString("结果元信息：\n")
		b.WriteString(fmt.Sprintf("- datasourceId: %s\n", stored.DatasourceID))
		if strings.TrimSpace(stored.Database) != "" {
			b.WriteString(fmt.Sprintf("- database: %s\n", stored.Database))
		}
		if strings.TrimSpace(stored.Statement) != "" {
			b.WriteString(fmt.Sprintf("- statement: %s\n", stored.Statement))
		}
		if stored.RowCount > 0 {
			b.WriteString(fmt.Sprintf("- rowCount: %d\n", stored.RowCount))
		}
		b.WriteString(fmt.Sprintf("- payloadRows: %d\n", len(stored.Rows)))
		if len(stored.Columns) > 0 {
			b.WriteString(fmt.Sprintf("- columns: %s\n", strings.Join(stored.Columns, ", ")))
		}
		if stored.RowsTruncated {
			b.WriteString("- truncated: true\n")
		} else {
			b.WriteString("- truncated: false\n")
		}
		b.WriteString("\nrows(JSON)：\n")
		b.Write(rowsJSON)
		return strings.TrimSpace(b.String())
	}

	b.WriteString("User visualization request:\n")
	b.WriteString(q + "\n\n")
	b.WriteString("Result metadata:\n")
	b.WriteString(fmt.Sprintf("- datasourceId: %s\n", stored.DatasourceID))
	if strings.TrimSpace(stored.Database) != "" {
		b.WriteString(fmt.Sprintf("- database: %s\n", stored.Database))
	}
	if strings.TrimSpace(stored.Statement) != "" {
		b.WriteString(fmt.Sprintf("- statement: %s\n", stored.Statement))
	}
	if stored.RowCount > 0 {
		b.WriteString(fmt.Sprintf("- rowCount: %d\n", stored.RowCount))
	}
	b.WriteString(fmt.Sprintf("- payloadRows: %d\n", len(stored.Rows)))
	if len(stored.Columns) > 0 {
		b.WriteString(fmt.Sprintf("- columns: %s\n", strings.Join(stored.Columns, ", ")))
	}
	if stored.RowsTruncated {
		b.WriteString("- truncated: true\n")
	} else {
		b.WriteString("- truncated: false\n")
	}
	b.WriteString("\nrows(JSON):\n")
	b.Write(rowsJSON)
	return strings.TrimSpace(b.String())
}

type visualizationModelOutput struct {
	Title    string `json:"title,omitempty"`
	Renderer string `json:"renderer,omitempty"`
	Spec     any    `json:"spec,omitempty"`
}

func parseVisualizationModelOutput(raw string) (visualizationModelOutput, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = stripCodeFence(trimmed)
	start := strings.Index(trimmed, "{")
	if start == -1 {
		return visualizationModelOutput{}, errors.New("no_json_object")
	}

	decoder := json.NewDecoder(strings.NewReader(trimmed[start:]))
	var payload json.RawMessage
	if err := decoder.Decode(&payload); err != nil {
		return visualizationModelOutput{}, err
	}

	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return visualizationModelOutput{}, err
	}

	// Guard against accidentally returning tool-protocol JSON.
	if _, ok := obj["assistantMessage"]; ok {
		return visualizationModelOutput{}, errors.New("unexpected_tool_protocol_json")
	}
	if _, ok := obj["toolCalls"]; ok {
		return visualizationModelOutput{}, errors.New("unexpected_tool_protocol_json")
	}

	out := visualizationModelOutput{
		Title: strings.TrimSpace(fmt.Sprint(obj["title"])),
		Spec:  obj["spec"],
	}

	renderer := strings.ToLower(strings.TrimSpace(fmt.Sprint(obj["renderer"])))
	if renderer == "" {
		renderer = strings.ToLower(strings.TrimSpace(fmt.Sprint(obj["type"])))
	}
	out.Renderer = renderer

	if out.Spec == nil {
		if opt, ok := obj["option"]; ok && opt != nil {
			out.Spec = opt
		}
	}

	out.Renderer = strings.TrimSpace(strings.ToLower(out.Renderer))
	if out.Renderer == "vega-lite" {
		out.Renderer = "vega_lite"
	}
	switch out.Renderer {
	case "echarts", "three", "vega_lite":
	default:
		return visualizationModelOutput{}, errors.New("renderer must be echarts|three|vega_lite")
	}
	if out.Spec == nil {
		return visualizationModelOutput{}, errors.New("spec is required")
	}
	return out, nil
}
