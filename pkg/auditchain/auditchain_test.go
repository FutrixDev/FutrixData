package auditchain

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestVerifyAcceptsLegacyPrefixAndChainedRows(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(`{"id":"legacy","toolName":"list_datasources"}` + "\n")

	row1, err := AddFields(map[string]any{"id": "a1", "status": "success"}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	writeJSONLine(t, &buf, row1)

	row2, err := AddFields(map[string]any{"id": "a2", "status": "error"}, 3, row1["chain_hash"].(string))
	if err != nil {
		t.Fatal(err)
	}
	writeJSONLine(t, &buf, row2)

	result, err := Verify(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pass || result.VerifiedRecords != 2 || result.LegacyRecords != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestVerifyDetectsTamperedPayload(t *testing.T) {
	row, err := AddFields(map[string]any{"id": "a1", "status": "success"}, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	row["status"] = "error"

	var buf bytes.Buffer
	writeJSONLine(t, &buf, row)
	result, err := Verify(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if result.Pass {
		t.Fatalf("expected failure")
	}
	if result.Reason != "payload hash mismatch" {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func writeJSONLine(t *testing.T, buf *bytes.Buffer, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	buf.Write(raw)
	buf.WriteByte('\n')
}
