package auditchain

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
)

const (
	Version     = "local-sha256-v1"
	GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"
)

const maxLineBytes = 8 * 1024 * 1024

var chainFields = map[string]struct{}{
	"seq":           {},
	"prev_hash":     {},
	"payload_hash":  {},
	"chain_hash":    {},
	"chain_version": {},
}

type VerifyResult struct {
	Pass                bool   `json:"pass"`
	VerifiedRecords     int    `json:"verified_records"`
	LegacyRecords       int    `json:"legacy_records"`
	TotalRecords        int    `json:"total_records"`
	FirstBrokenPosition int    `json:"first_broken_position,omitempty"`
	Reason              string `json:"reason,omitempty"`
	ExpectedHash        string `json:"expected_hash,omitempty"`
	ActualHash          string `json:"actual_hash,omitempty"`
	Source              string `json:"source"`
	Path                string `json:"path,omitempty"`
}

type lineKind int

const (
	lineLegacy lineKind = iota
	lineChained
	linePartial
)

func AddFields(record map[string]any, seq int64, prevHash string) (map[string]any, error) {
	if seq <= 0 {
		return nil, fmt.Errorf("audit chain seq must be positive")
	}
	next := make(map[string]any, len(record)+5)
	for k, v := range record {
		if _, reserved := chainFields[k]; reserved {
			continue
		}
		next[k] = v
	}
	if strings.TrimSpace(prevHash) == "" {
		prevHash = GenesisHash
	}
	next["seq"] = seq
	next["prev_hash"] = strings.TrimSpace(prevHash)
	next["chain_version"] = Version

	payloadHash, err := PayloadHash(next)
	if err != nil {
		return nil, err
	}
	next["payload_hash"] = payloadHash
	chainHash, err := ChainHash(seq, strings.TrimSpace(prevHash), payloadHash, Version)
	if err != nil {
		return nil, err
	}
	next["chain_hash"] = chainHash
	return next, nil
}

func VerifyFile(path string) (VerifyResult, error) {
	path = strings.TrimSpace(path)
	result := VerifyResult{Pass: true, Source: "file", Path: path}
	if path == "" {
		result.Pass = false
		result.Reason = "missing audit file path"
		return result, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return VerifyResult{}, err
	}
	defer f.Close()

	result, err = Verify(f)
	result.Source = "file"
	result.Path = path
	return result, err
}

func Verify(r io.Reader) (VerifyResult, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return VerifyResult{}, err
	}
	return verifyData(data), nil
}

func verifyData(data []byte) VerifyResult {
	result := VerifyResult{Pass: true, Source: "reader"}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	var position int
	chainStarted := false
	expectedPrev := GenesisHash

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		position++
		result.TotalRecords = position

		raw, err := decodeLine(line)
		if err != nil {
			return fail(result, position, "invalid JSON audit row: "+err.Error(), "", "")
		}

		switch classify(raw) {
		case lineLegacy:
			if chainStarted {
				return fail(result, position, "legacy audit row found after hash chain started", "", "")
			}
			result.LegacyRecords++
			continue
		case linePartial:
			return fail(result, position, "incomplete hash-chain fields", "", "")
		case lineChained:
		}

		seq, err := int64From(raw, "seq")
		if err != nil || seq <= 0 {
			return fail(result, position, "invalid hash-chain sequence", "", "")
		}
		if seq != int64(position) {
			return fail(result, position, fmt.Sprintf("unexpected hash-chain sequence: got %d want %d", seq, position), "", "")
		}

		version := strings.TrimSpace(stringFrom(raw, "chain_version"))
		if version != Version {
			return fail(result, position, "unsupported hash-chain version", "", "")
		}
		prevHash := strings.TrimSpace(stringFrom(raw, "prev_hash"))
		if prevHash != expectedPrev {
			return fail(result, position, "previous hash mismatch", expectedPrev, prevHash)
		}

		expectedPayload, err := payloadHashForLine(line)
		if err != nil {
			return fail(result, position, "cannot hash audit payload", "", "")
		}
		actualPayload := strings.TrimSpace(stringFrom(raw, "payload_hash"))
		if actualPayload != expectedPayload {
			return fail(result, position, "payload hash mismatch", expectedPayload, actualPayload)
		}

		expectedChain, err := ChainHash(seq, prevHash, expectedPayload, version)
		if err != nil {
			return fail(result, position, "cannot hash audit chain", "", "")
		}
		actualChain := strings.TrimSpace(stringFrom(raw, "chain_hash"))
		if actualChain != expectedChain {
			return fail(result, position, "chain hash mismatch", expectedChain, actualChain)
		}

		chainStarted = true
		expectedPrev = actualChain
		result.VerifiedRecords++
	}
	if err := scanner.Err(); err != nil {
		return fail(result, position+1, err.Error(), "", "")
	}
	return result
}

func PayloadHash(record map[string]any) (string, error) {
	next := make(map[string]any, len(record))
	for k, v := range record {
		if _, reserved := chainFields[k]; reserved {
			continue
		}
		next[k] = v
	}
	canonical, err := json.Marshal(next)
	if err != nil {
		return "", err
	}
	return sha256Hex(canonical), nil
}

func ChainHash(seq int64, prevHash, payloadHash, version string) (string, error) {
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

func payloadHashForLine(line []byte) (string, error) {
	raw, err := decodeLine(line)
	if err != nil {
		return "", err
	}
	return PayloadHash(raw)
}

func decodeLine(line []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func classify(raw map[string]any) lineKind {
	present := 0
	for field := range chainFields {
		if _, ok := raw[field]; ok {
			present++
		}
	}
	if present == 0 {
		return lineLegacy
	}
	if present != len(chainFields) {
		return linePartial
	}
	return lineChained
}

func int64From(raw map[string]any, field string) (int64, error) {
	v, ok := raw[field]
	if !ok || v == nil {
		return 0, fmt.Errorf("missing %s", field)
	}
	switch t := v.(type) {
	case json.Number:
		return t.Int64()
	case float64:
		return int64(t), nil
	case int64:
		return t, nil
	case int:
		return int64(t), nil
	case string:
		return strconv.ParseInt(strings.TrimSpace(t), 10, 64)
	default:
		return 0, fmt.Errorf("invalid %s", field)
	}
}

func stringFrom(raw map[string]any, field string) string {
	if v, ok := raw[field]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

func fail(result VerifyResult, position int, reason, expected, actual string) VerifyResult {
	result.Pass = false
	result.FirstBrokenPosition = position
	result.Reason = reason
	result.ExpectedHash = expected
	result.ActualHash = actual
	return result
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
