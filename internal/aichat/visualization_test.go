package aichat

import (
	"strings"
	"testing"
)

func TestParseVisualizationModelOutput_VegaLite(t *testing.T) {
	t.Run("accepts vega_lite", func(t *testing.T) {
		raw := `{"title":"Demo","renderer":"vega_lite","spec":{"$schema":"https://vega.github.io/schema/vega-lite/v5.json","data":{"values":[{"name":"A","value":1},{"name":"B","value":2}]},"mark":"bar","encoding":{"x":{"field":"name","type":"nominal"},"y":{"field":"value","type":"quantitative"}}}}`

		out, err := parseVisualizationModelOutput(raw)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if out.Renderer != "vega_lite" {
			t.Fatalf("expected renderer vega_lite, got %q", out.Renderer)
		}
		if out.Spec == nil {
			t.Fatalf("expected spec to be present")
		}
		if out.Title != "Demo" {
			t.Fatalf("expected title Demo, got %q", out.Title)
		}
	})

	t.Run("accepts vega-lite and normalizes", func(t *testing.T) {
		raw := `{"title":"Demo","renderer":"vega-lite","spec":{"$schema":"https://vega.github.io/schema/vega-lite/v5.json","data":{"values":[{"name":"A","value":1}]},"mark":"bar","encoding":{"x":{"field":"name","type":"nominal"},"y":{"field":"value","type":"quantitative"}}}}`

		out, err := parseVisualizationModelOutput(raw)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if out.Renderer != "vega_lite" {
			t.Fatalf("expected renderer vega_lite, got %q", out.Renderer)
		}
	})
}

func TestVisualizationSystemPrompt_IncludesVegaLite(t *testing.T) {
	promptEN := visualizationSystemPrompt(uiLocaleEN)
	if !strings.Contains(promptEN, "vega_lite") {
		t.Fatalf("expected EN prompt to mention vega_lite renderer")
	}
	promptZH := visualizationSystemPrompt(uiLocaleZH)
	if !strings.Contains(promptZH, "vega_lite") {
		t.Fatalf("expected ZH prompt to mention vega_lite renderer")
	}
}
