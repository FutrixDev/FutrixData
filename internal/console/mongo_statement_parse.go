package console

import (
	"encoding/json"
	"errors"
	"strings"
)

func parseMongoStatement(statement string) (mongoStatement, error) {
	trimmed := strings.TrimSpace(statement)
	trimmed = stripMongoMarkdown(trimmed)
	trimmed = strings.TrimSpace(trimmed)
	if strings.HasPrefix(trimmed, "{") {
		return parseMongoJSONStatement(trimmed)
	}
	call, err := parseMongoCall(trimmed)
	if err != nil {
		return mongoStatement{}, err
	}
	return mongoStatementFromCall(call)
}

func stripMongoMarkdown(statement string) string {
	trimmed := strings.TrimSpace(statement)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimSpace(trimmed[3:])
		if trimmed == "" {
			return ""
		}
		if idx := strings.Index(trimmed, "\n"); idx != -1 {
			header := strings.TrimSpace(trimmed[:idx])
			if isFenceInfoToken(header) {
				trimmed = trimmed[idx+1:]
			}
		}
		if end := strings.LastIndex(trimmed, "```"); end != -1 {
			trimmed = trimmed[:end]
		}
		trimmed = strings.TrimSpace(trimmed)
	}

	if len(trimmed) >= 2 && strings.HasPrefix(trimmed, "`") && strings.HasSuffix(trimmed, "`") {
		trimmed = strings.TrimSuffix(strings.TrimPrefix(trimmed, "`"), "`")
		trimmed = strings.TrimSpace(trimmed)
	}

	if strings.HasSuffix(trimmed, "```") {
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "```"))
	}

	// Tolerate malformed Markdown fences (e.g. trailing ``) that can appear in LLM output.
	trimmed = strings.TrimRight(trimmed, "`")
	return strings.TrimSpace(trimmed)
}

func isFenceInfoToken(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		ch := value[i]
		isAlpha := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
		isDigit := ch >= '0' && ch <= '9'
		isAllowed := ch == '_' || ch == '-' || ch == '+'
		if !(isAlpha || isDigit || isAllowed) {
			return false
		}
	}
	return true
}

func parseMongoJSONStatement(statement string) (mongoStatement, error) {
	type mongoJSONStatement struct {
		Database   string         `json:"database"`
		Collection string         `json:"collection"`
		Action     string         `json:"action"`
		Filter     map[string]any `json:"filter"`
		Document   any            `json:"document"`
		Update     any            `json:"update"`
		Pipeline   []any          `json:"pipeline"`
		Keys       map[string]any `json:"keys"`
		Options    map[string]any `json:"options"`
		Limit      int64          `json:"limit"`

		Sort       any `json:"sort"`
		Projection any `json:"projection"`
		Skip       any `json:"skip"`
		Hint       any `json:"hint"`
	}

	var payload mongoJSONStatement
	if err := json.Unmarshal([]byte(statement), &payload); err != nil {
		return mongoStatement{}, err
	}

	stmt := mongoStatement{
		Database:   payload.Database,
		Collection: payload.Collection,
		Action:     payload.Action,
		Filter:     payload.Filter,
		Document:   payload.Document,
		Update:     payload.Update,
		Pipeline:   payload.Pipeline,
		Keys:       payload.Keys,
		Options:    payload.Options,
		Limit:      payload.Limit,
	}
	stmt.Action = normalizeMongoAction(stmt.Action)
	if (stmt.Action == "replaceone" || stmt.Action == "findoneandreplace") && stmt.Update == nil && payload.Document != nil {
		stmt.Update = payload.Document
	}
	if stmt.Collection == "" {
		return stmt, errors.New("collection is required")
	}
	ensureMongoDefaults(&stmt)
	if payload.Sort != nil && stmt.Options["sort"] == nil {
		stmt.Options["sort"] = payload.Sort
	}
	if payload.Projection != nil && stmt.Options["projection"] == nil {
		stmt.Options["projection"] = payload.Projection
	}
	if payload.Skip != nil && stmt.Options["skip"] == nil {
		stmt.Options["skip"] = payload.Skip
	}
	if payload.Hint != nil && stmt.Options["hint"] == nil {
		stmt.Options["hint"] = payload.Hint
	}
	return stmt, nil
}

func mongoStatementFromCall(call mongoCall) (mongoStatement, error) {
	stmt := mongoStatement{Database: call.Database, Collection: call.Collection}
	action := normalizeMongoAction(call.Method)

	switch action {
	case "find":
		stmt.Action = action
		stmt.Filter = argAsMap(call.Args, 0)
		stmt.Options = argAsMap(call.Args, 1)
		if limit, ok := int64From(stmt.Options["limit"]); ok {
			stmt.Limit = limit
		}
		ensureMongoDefaults(&stmt)
		return stmt, nil
	case "aggregate":
		stmt.Action = action
		stmt.Pipeline = argAsSlice(call.Args, 0)
		ensureMongoDefaults(&stmt)
		return stmt, nil
	case "insert":
		stmt.Action = "insertone"
		if doc := argValue(call.Args, 0); doc != nil {
			switch doc.(type) {
			case []any:
				stmt.Action = "insertmany"
			}
			stmt.Document = doc
		}
		return stmt, nil
	case "insertone", "insertmany":
		stmt.Action = action
		stmt.Document = argValue(call.Args, 0)
		return stmt, nil
	case "update":
		stmt.Action = "updatemany"
		stmt.Filter = argAsMap(call.Args, 0)
		stmt.Update = argValue(call.Args, 1)
		ensureMongoDefaults(&stmt)
		return stmt, nil
	case "updateone", "updatemany":
		stmt.Action = action
		stmt.Filter = argAsMap(call.Args, 0)
		stmt.Update = argValue(call.Args, 1)
		stmt.Options = argAsMap(call.Args, 2)
		ensureMongoDefaults(&stmt)
		return stmt, nil
	case "replaceone":
		stmt.Action = action
		stmt.Filter = argAsMap(call.Args, 0)
		stmt.Update = argValue(call.Args, 1)
		stmt.Options = argAsMap(call.Args, 2)
		ensureMongoDefaults(&stmt)
		return stmt, nil
	case "delete":
		stmt.Action = "deletemany"
		stmt.Filter = argAsMap(call.Args, 0)
		ensureMongoDefaults(&stmt)
		return stmt, nil
	case "deleteone", "deletemany", "remove":
		stmt.Action = action
		if action == "remove" {
			stmt.Action = "deletemany"
		}
		stmt.Filter = argAsMap(call.Args, 0)
		stmt.Options = argAsMap(call.Args, 1)
		ensureMongoDefaults(&stmt)
		return stmt, nil
	case "findoneandupdate":
		stmt.Action = action
		stmt.Filter = argAsMap(call.Args, 0)
		stmt.Update = argValue(call.Args, 1)
		stmt.Options = argAsMap(call.Args, 2)
		ensureMongoDefaults(&stmt)
		return stmt, nil
	case "findoneandreplace":
		stmt.Action = action
		stmt.Filter = argAsMap(call.Args, 0)
		stmt.Update = argValue(call.Args, 1)
		stmt.Options = argAsMap(call.Args, 2)
		ensureMongoDefaults(&stmt)
		return stmt, nil
	case "findoneanddelete":
		stmt.Action = action
		stmt.Filter = argAsMap(call.Args, 0)
		stmt.Options = argAsMap(call.Args, 1)
		ensureMongoDefaults(&stmt)
		return stmt, nil
	case "bulkwrite":
		stmt.Action = action
		stmt.Document = argValue(call.Args, 0)
		stmt.Options = argAsMap(call.Args, 1)
		return stmt, nil
	case "createcollection":
		stmt.Action = action
		if stmt.Collection == "" {
			if name, ok := argString(call.Args, 0); ok {
				stmt.Collection = name
			}
		}
		if stmt.Collection == "" {
			return mongoStatement{}, errors.New("collection is required")
		}
		return stmt, nil
	case "drop":
		stmt.Action = action
		if stmt.Collection == "" {
			if name, ok := argString(call.Args, 0); ok {
				stmt.Collection = name
			}
		}
		if stmt.Collection == "" {
			return mongoStatement{}, errors.New("collection is required")
		}
		return stmt, nil
	case "createuser":
		stmt.Action = action
		stmt.Document = argValue(call.Args, 0)
		if stmt.Document == nil {
			return mongoStatement{}, errors.New("user document is required")
		}
		return stmt, nil
	case "getusers":
		stmt.Action = action
		stmt.Options = argAsMap(call.Args, 0)
		return stmt, nil
	case "createindex":
		stmt.Action = action
		stmt.Keys = argAsMap(call.Args, 0)
		stmt.Options = argAsMap(call.Args, 1)
		if len(stmt.Keys) == 0 {
			return mongoStatement{}, errors.New("keys is required for createIndex")
		}
		ensureMongoDefaults(&stmt)
		return stmt, nil
	case "dropindex":
		stmt.Action = action
		stmt.Options = map[string]any{}
		if name, ok := argString(call.Args, 0); ok {
			stmt.Options["name"] = name
		}
		if stmt.Options["name"] == "" {
			return mongoStatement{}, errors.New("index name is required")
		}
		return stmt, nil
	case "serverstatus":
		stmt.Action = action
		stmt.Options = argAsMap(call.Args, 0)
		return stmt, nil
	default:
		return mongoStatement{}, ErrUnsupported
	}
}

func normalizeMongoAction(action string) string {
	value := strings.ToLower(strings.TrimSpace(action))
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

func ensureMongoDefaults(stmt *mongoStatement) {
	if stmt.Filter == nil {
		stmt.Filter = map[string]any{}
	}
	if stmt.Pipeline == nil {
		stmt.Pipeline = []any{}
	}
	if stmt.Keys == nil {
		stmt.Keys = map[string]any{}
	}
	if stmt.Options == nil {
		stmt.Options = map[string]any{}
	}
}

func argValue(args []any, idx int) any {
	if idx < 0 || idx >= len(args) {
		return nil
	}
	return args[idx]
}

func argAsMap(args []any, idx int) map[string]any {
	if idx < 0 || idx >= len(args) {
		return map[string]any{}
	}
	if value, ok := args[idx].(map[string]any); ok {
		return value
	}
	return map[string]any{}
}

func argAsSlice(args []any, idx int) []any {
	if idx < 0 || idx >= len(args) {
		return []any{}
	}
	if value, ok := args[idx].([]any); ok {
		return value
	}
	return []any{}
}

func argString(args []any, idx int) (string, bool) {
	if idx < 0 || idx >= len(args) {
		return "", false
	}
	value, ok := args[idx].(string)
	return value, ok
}
