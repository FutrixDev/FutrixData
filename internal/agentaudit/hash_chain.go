package agentaudit

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"futrixdata/platform/internal/securefile"
)

const (
	AuditChainVersion     = "local-sha256-v1"
	AuditChainGenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"
)

var auditChainFields = map[string]struct{}{
	"seq":           {},
	"prev_hash":     {},
	"payload_hash":  {},
	"chain_hash":    {},
	"chain_version": {},
}

type auditChainAppendState struct {
	totalRecords  int64
	chainStarted  bool
	lastChainHash string
}

type auditLineKind int

const (
	auditLineLegacy auditLineKind = iota
	auditLineChained
	auditLinePartial
)

func VerifyFile(path string) (VerifyResult, error) {
	result := VerifyResult{
		Pass:   true,
		Source: "file",
		Path:   strings.TrimSpace(path),
	}
	if strings.TrimSpace(path) == "" {
		result.Pass = false
		result.Reason = "missing audit file path"
		return result, nil
	}
	err := securefile.WithPathLock(path, func() error {
		data, err := securefile.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		result = verifyAuditData(path, data)
		return nil
	})
	if err != nil {
		return VerifyResult{}, err
	}
	return result, nil
}

func addHashChain(entry AuditEntry, seq int64, prevHash string) (AuditEntry, error) {
	if seq <= 0 {
		return AuditEntry{}, fmt.Errorf("audit chain seq must be positive")
	}
	trimmedPrev := strings.TrimSpace(prevHash)
	if trimmedPrev == "" {
		trimmedPrev = AuditChainGenesisHash
	}
	entry.Seq = seq
	entry.PrevHash = trimmedPrev
	entry.ChainVersion = AuditChainVersion
	entry.PayloadHash = ""
	entry.ChainHash = ""

	payloadHash, err := payloadHashForEntry(entry)
	if err != nil {
		return AuditEntry{}, err
	}
	entry.PayloadHash = payloadHash
	chainHash, err := computeChainHash(entry.Seq, entry.PrevHash, entry.PayloadHash, entry.ChainVersion)
	if err != nil {
		return AuditEntry{}, err
	}
	entry.ChainHash = chainHash
	return entry, nil
}

func scanAuditChainForAppend(data []byte) (auditChainAppendState, error) {
	result := verifyAuditData("", data)
	if !result.Pass {
		return auditChainAppendState{}, fmt.Errorf("audit chain verification failed at record %d: %s", result.FirstBrokenPosition, result.Reason)
	}
	state := auditChainAppendState{totalRecords: int64(result.TotalRecords)}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), maxAuditLineBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		raw, err := decodeAuditLine(line)
		if err != nil {
			return auditChainAppendState{}, err
		}
		if classifyAuditLine(raw) == auditLineChained {
			state.chainStarted = true
			state.lastChainHash = strings.TrimSpace(stringValueFromMap(raw, "chain_hash"))
		}
	}
	if err := scanner.Err(); err != nil {
		return auditChainAppendState{}, err
	}
	return state, nil
}

func verifyAuditData(path string, data []byte) VerifyResult {
	result := VerifyResult{
		Pass:   true,
		Source: "file",
		Path:   strings.TrimSpace(path),
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), maxAuditLineBytes)

	var position int
	var chainStarted bool
	expectedPrev := AuditChainGenesisHash
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		position++
		result.TotalRecords = position
		raw, err := decodeAuditLine(line)
		if err != nil {
			return failVerify(result, position, "invalid JSON audit row: "+err.Error(), "", "")
		}
		switch classifyAuditLine(raw) {
		case auditLineLegacy:
			if chainStarted {
				return failVerify(result, position, "legacy audit row found after hash chain started", "", "")
			}
			result.LegacyRecords++
			continue
		case auditLinePartial:
			return failVerify(result, position, "incomplete hash-chain fields", "", "")
		case auditLineChained:
		}

		seq, err := int64ValueFromMap(raw, "seq")
		if err != nil || seq <= 0 {
			return failVerify(result, position, "invalid hash-chain sequence", "", "")
		}
		if seq != int64(position) {
			return failVerify(result, position, fmt.Sprintf("unexpected hash-chain sequence: got %d want %d", seq, position), "", "")
		}
		version := strings.TrimSpace(stringValueFromMap(raw, "chain_version"))
		if version != AuditChainVersion {
			return failVerify(result, position, "unsupported hash-chain version", "", "")
		}
		prevHash := strings.TrimSpace(stringValueFromMap(raw, "prev_hash"))
		if prevHash != expectedPrev {
			return failVerify(result, position, "previous hash mismatch", expectedPrev, prevHash)
		}

		expectedPayloadHash, err := payloadHashForRawLine(line)
		if err != nil {
			return failVerify(result, position, "cannot hash audit payload", "", "")
		}
		actualPayloadHash := strings.TrimSpace(stringValueFromMap(raw, "payload_hash"))
		if actualPayloadHash != expectedPayloadHash {
			return failVerify(result, position, "payload hash mismatch", expectedPayloadHash, actualPayloadHash)
		}
		expectedChainHash, err := computeChainHash(seq, prevHash, expectedPayloadHash, version)
		if err != nil {
			return failVerify(result, position, "cannot hash audit chain", "", "")
		}
		actualChainHash := strings.TrimSpace(stringValueFromMap(raw, "chain_hash"))
		if actualChainHash != expectedChainHash {
			return failVerify(result, position, "chain hash mismatch", expectedChainHash, actualChainHash)
		}

		chainStarted = true
		expectedPrev = actualChainHash
		result.VerifiedRecords++
	}
	if err := scanner.Err(); err != nil {
		return failVerify(result, position+1, err.Error(), "", "")
	}
	return result
}

func failVerify(result VerifyResult, position int, reason, expected, actual string) VerifyResult {
	result.Pass = false
	result.FirstBrokenPosition = position
	result.Reason = reason
	result.ExpectedHash = expected
	result.ActualHash = actual
	return result
}

func classifyAuditLine(raw map[string]any) auditLineKind {
	present := 0
	for field := range auditChainFields {
		if _, ok := raw[field]; ok {
			present++
		}
	}
	if present == 0 {
		return auditLineLegacy
	}
	if present != len(auditChainFields) {
		return auditLinePartial
	}
	return auditLineChained
}

func payloadHashForEntry(entry AuditEntry) (string, error) {
	payload, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	return payloadHashForRawLine(payload)
}

func payloadHashForRawLine(line []byte) (string, error) {
	raw, err := decodeAuditLine(line)
	if err != nil {
		return "", err
	}
	for field := range auditChainFields {
		delete(raw, field)
	}
	canonical, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	return sha256Hex(canonical), nil
}

func computeChainHash(seq int64, prevHash, payloadHash, version string) (string, error) {
	canonical, err := json.Marshal(map[string]any{
		"chain_version": strings.TrimSpace(version),
		"payload_hash":  strings.TrimSpace(payloadHash),
		"prev_hash":     strings.TrimSpace(prevHash),
		"seq":           seq,
	})
	if err != nil {
		return "", err
	}
	return sha256Hex(canonical), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func decodeAuditLine(line []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	value, err := decodeJSONValue(dec)
	if err != nil {
		return nil, err
	}
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("audit row must be a JSON object")
	}
	if _, err := dec.Token(); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("multiple JSON values in audit row")
	}
	return raw, nil
}

func decodeJSONValue(dec *json.Decoder) (any, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch typed := token.(type) {
	case json.Delim:
		switch typed {
		case '{':
			obj := make(map[string]any)
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("object key must be a string")
				}
				if _, exists := obj[key]; exists {
					return nil, fmt.Errorf("duplicate JSON object key %q", key)
				}
				value, err := decodeJSONValue(dec)
				if err != nil {
					return nil, err
				}
				obj[key] = value
			}
			end, err := dec.Token()
			if err != nil {
				return nil, err
			}
			if end != json.Delim('}') {
				return nil, fmt.Errorf("expected end of object")
			}
			return obj, nil
		case '[':
			items := make([]any, 0)
			for dec.More() {
				value, err := decodeJSONValue(dec)
				if err != nil {
					return nil, err
				}
				items = append(items, value)
			}
			end, err := dec.Token()
			if err != nil {
				return nil, err
			}
			if end != json.Delim(']') {
				return nil, fmt.Errorf("expected end of array")
			}
			return items, nil
		default:
			return nil, fmt.Errorf("unexpected JSON delimiter %q", typed)
		}
	default:
		return typed, nil
	}
}

func stringValueFromMap(raw map[string]any, key string) string {
	value, ok := raw[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func int64ValueFromMap(raw map[string]any, key string) (int64, error) {
	value, ok := raw[key]
	if !ok || value == nil {
		return 0, fmt.Errorf("missing %s", key)
	}
	switch typed := value.(type) {
	case json.Number:
		return typed.Int64()
	case float64:
		return int64(typed), nil
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case string:
		return strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
	default:
		return 0, fmt.Errorf("invalid %s", key)
	}
}
