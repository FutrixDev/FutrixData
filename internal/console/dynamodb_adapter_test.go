package console

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"futrixdata/platform/internal/datasource"
)

func TestDynamoDBAdapter_TestConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if target := r.Header.Get("X-Amz-Target"); target != "DynamoDB_20120810.ListTables" {
			t.Fatalf("expected ListTables target, got %q", target)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"TableNames": []string{}})
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	if err := adapter.TestConnection(context.Background(), ds); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
}

func TestDynamoDBAdapter_ListEntities(t *testing.T) {
	listCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		target := r.Header.Get("X-Amz-Target")
		switch target {
		case "DynamoDB_20120810.ListTables":
			listCalls++
			if listCalls == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"TableNames":             []string{"z-last", "futrixdata-demo-1"},
					"LastEvaluatedTableName": "continue",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"TableNames": []string{"futrixdata-demo-2", "a-first"},
			})
			return
		default:
			t.Fatalf("unexpected target %q", target)
		}
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	items, err := adapter.ListEntities(context.Background(), ds, ListOptions{Pattern: "demo"})
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 tables, got %d (%v)", len(items), items)
	}
	if items[0] != "futrixdata-demo-1" || items[1] != "futrixdata-demo-2" {
		t.Fatalf("unexpected tables: %v", items)
	}
}

func TestDynamoDBAdapter_ListEntitiesPage(t *testing.T) {
	type listTablesPayload struct {
		ExclusiveStartTableName *string `json:"ExclusiveStartTableName"`
		Limit                   *int32  `json:"Limit"`
	}

	call := 0
	requests := make([]listTablesPayload, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		target := r.Header.Get("X-Amz-Target")
		switch target {
		case "DynamoDB_20120810.ListTables":
			call++
			var payload listTablesPayload
			_ = json.NewDecoder(r.Body).Decode(&payload)
			requests = append(requests, payload)

			switch call {
			case 1:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"TableNames":             []string{"a-first", "b-next"},
					"LastEvaluatedTableName": "b-next",
				})
				return
			case 2:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"TableNames": []string{"c-last"},
				})
				return
			default:
				t.Fatalf("unexpected ListTables call %d", call)
			}
		default:
			t.Fatalf("unexpected target %q", target)
		}
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	page1, err := adapter.ListEntitiesPage(context.Background(), ds, ListOptions{Limit: 2}, "")
	if err != nil {
		t.Fatalf("ListEntitiesPage: %v", err)
	}
	if len(page1.Items) != 2 || page1.Items[0] != "a-first" || page1.Items[1] != "b-next" {
		t.Fatalf("unexpected items: %v", page1.Items)
	}
	if page1.Cursor != "b-next" || page1.Done {
		t.Fatalf("expected cursor=b-next done=false, got cursor=%q done=%v", page1.Cursor, page1.Done)
	}

	page2, err := adapter.ListEntitiesPage(context.Background(), ds, ListOptions{Limit: 2}, page1.Cursor)
	if err != nil {
		t.Fatalf("ListEntitiesPage: %v", err)
	}
	if len(page2.Items) != 1 || page2.Items[0] != "c-last" {
		t.Fatalf("unexpected page2 items: %v", page2.Items)
	}
	if page2.Cursor != "" || !page2.Done {
		t.Fatalf("expected cursor='' done=true, got cursor=%q done=%v", page2.Cursor, page2.Done)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	if requests[0].Limit == nil || *requests[0].Limit != 2 || requests[0].ExclusiveStartTableName != nil {
		t.Fatalf("unexpected first request: %#v", requests[0])
	}
	if requests[1].Limit == nil || *requests[1].Limit != 2 || requests[1].ExclusiveStartTableName == nil || *requests[1].ExclusiveStartTableName != "b-next" {
		t.Fatalf("unexpected second request: %#v", requests[1])
	}
}

func TestDynamoDBAdapter_ListEntitiesPage_PatternSearchScansAhead(t *testing.T) {
	type listTablesPayload struct {
		ExclusiveStartTableName *string `json:"ExclusiveStartTableName"`
		Limit                   *int32  `json:"Limit"`
	}

	call := 0
	requests := make([]listTablesPayload, 0, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if target := r.Header.Get("X-Amz-Target"); target != "DynamoDB_20120810.ListTables" {
			t.Fatalf("unexpected target %q", target)
		}
		call++
		var payload listTablesPayload
		_ = json.NewDecoder(r.Body).Decode(&payload)
		requests = append(requests, payload)

		switch call {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"TableNames":             []string{"alpha", "bravo"},
				"LastEvaluatedTableName": "bravo",
			})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"TableNames":             []string{"demo-1", "echo"},
				"LastEvaluatedTableName": "echo",
			})
		case 3:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"TableNames": []string{"foxtrot", "demo-2"},
			})
		default:
			t.Fatalf("unexpected ListTables call %d", call)
		}
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	page, err := adapter.ListEntitiesPage(context.Background(), ds, ListOptions{Limit: 2, Pattern: "demo"}, "")
	if err != nil {
		t.Fatalf("ListEntitiesPage: %v", err)
	}
	if len(page.Items) != 2 || page.Items[0] != "demo-1" || page.Items[1] != "demo-2" {
		t.Fatalf("unexpected items: %v", page.Items)
	}
	if page.Cursor != "" || !page.Done {
		t.Fatalf("expected cursor='' done=true, got cursor=%q done=%v", page.Cursor, page.Done)
	}

	if len(requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(requests))
	}
	if requests[0].Limit == nil || *requests[0].Limit != 2 || requests[0].ExclusiveStartTableName != nil {
		t.Fatalf("unexpected first request: %#v", requests[0])
	}
	if requests[1].ExclusiveStartTableName == nil || *requests[1].ExclusiveStartTableName != "bravo" {
		t.Fatalf("unexpected second request: %#v", requests[1])
	}
	if requests[2].ExclusiveStartTableName == nil || *requests[2].ExclusiveStartTableName != "echo" {
		t.Fatalf("unexpected third request: %#v", requests[2])
	}
}

func TestDynamoDBAdapter_DescribeEntity(t *testing.T) {
	table := "orders"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		target := r.Header.Get("X-Amz-Target")
		switch target {
		case "DynamoDB_20120810.DescribeTable":
			raw, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(raw), fmt.Sprintf(`"TableName":"%s"`, table)) {
				t.Fatalf("expected request TableName=%q, got %s", table, string(raw))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Table": map[string]any{
					"TableName":   table,
					"TableStatus": "ACTIVE",
					"ItemCount":   3,
					"KeySchema": []map[string]any{
						{"AttributeName": "pk", "KeyType": "HASH"},
						{"AttributeName": "sk", "KeyType": "RANGE"},
					},
					"AttributeDefinitions": []map[string]any{
						{"AttributeName": "pk", "AttributeType": "S"},
						{"AttributeName": "sk", "AttributeType": "S"},
					},
					"GlobalSecondaryIndexes": []map[string]any{
						{
							"IndexName": "gsi_owner",
							"KeySchema": []map[string]any{
								{"AttributeName": "ownerId", "KeyType": "HASH"},
							},
							"Projection": map[string]any{"ProjectionType": "ALL"},
						},
					},
				},
			})
			return
		default:
			t.Fatalf("unexpected target %q", target)
		}
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	result, err := adapter.DescribeEntity(context.Background(), ds, table)
	if err != nil {
		t.Fatalf("DescribeEntity: %v", err)
	}
	if len(result.Columns) < 2 {
		t.Fatalf("expected columns, got %v", result.Columns)
	}
	if len(result.Indexes) < 1 {
		t.Fatalf("expected indexes, got %v", result.Indexes)
	}
	pk := ""
	sk := ""
	for _, item := range result.Details {
		switch item.Label {
		case "Partition Key":
			pk = fmt.Sprint(item.Value)
		case "Sort Key":
			sk = fmt.Sprint(item.Value)
		}
	}
	if pk != "pk" {
		t.Fatalf("expected partition key pk, got %q", pk)
	}
	if sk != "sk" {
		t.Fatalf("expected sort key sk, got %q", sk)
	}
}

func TestDynamoDBAdapter_Execute_Select(t *testing.T) {
	type executePayload struct {
		Statement string `json:"Statement"`
		Limit     *int32 `json:"Limit"`
	}
	payloadCh := make(chan executePayload, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		target := r.Header.Get("X-Amz-Target")
		switch target {
		case "DynamoDB_20120810.ExecuteStatement":
			var payload executePayload
			_ = json.NewDecoder(r.Body).Decode(&payload)
			select {
			case payloadCh <- payload:
			default:
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Items": []map[string]any{
					{
						"pk":     map[string]any{"S": "USER#1"},
						"count":  map[string]any{"N": "42"},
						"active": map[string]any{"BOOL": true},
						"meta": map[string]any{"M": map[string]any{
							"region": map[string]any{"S": "us-east-1"},
						}},
					},
					{
						"pk":    map[string]any{"S": "USER#2"},
						"count": map[string]any{"N": "7"},
					},
				},
				"NextToken": "token2",
			})
			return
		default:
			t.Fatalf("unexpected target %q", target)
		}
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	result, err := adapter.Execute(context.Background(), ds, `SELECT * FROM "orders" LIMIT 2`, ExecuteOptions{PageSize: 100})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	payload := <-payloadCh
	if payload.Statement != `SELECT * FROM "orders"` {
		t.Fatalf("expected stripped statement, got %q", payload.Statement)
	}
	if payload.Limit == nil || *payload.Limit != 2 {
		t.Fatalf("expected Limit 2, got %#v", payload.Limit)
	}
	if result.RowCount != 2 || len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got rowCount=%d rows=%d", result.RowCount, len(result.Rows))
	}
	if got := fmt.Sprint(result.Rows[0]["pk"]); got != "USER#1" {
		t.Fatalf("expected pk USER#1, got %q", got)
	}
	if got := fmt.Sprint(result.Rows[0]["count"]); got != "42" {
		t.Fatalf("expected count 42, got %q", got)
	}
	if got := result.Rows[0]["active"]; got != true {
		t.Fatalf("expected active true, got %#v", got)
	}
	if meta, ok := result.Rows[0]["meta"].(map[string]any); !ok || fmt.Sprint(meta["region"]) != "us-east-1" {
		t.Fatalf("expected meta.region us-east-1, got %#v", result.Rows[0]["meta"])
	}
	if result.NextToken != "token2" {
		t.Fatalf("expected next token, got %q", result.NextToken)
	}
	if !result.HasMore {
		t.Fatalf("expected HasMore true")
	}
}

func TestDynamoDBAdapter_Execute_AutoRepairsSingleQuotedTarget(t *testing.T) {
	type executePayload struct {
		Statement string `json:"Statement"`
	}
	payloadCh := make(chan executePayload, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch target := r.Header.Get("X-Amz-Target"); target {
		case "DynamoDB_20120810.DescribeTable":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Table": map[string]any{
					"TableName":   "log_issue",
					"TableStatus": "ACTIVE",
					"KeySchema": []map[string]any{
						{"AttributeName": "aid", "KeyType": "HASH"},
					},
					"AttributeDefinitions": []map[string]any{
						{"AttributeName": "aid", "AttributeType": "S"},
					},
				},
			})
			return
		case "DynamoDB_20120810.ExecuteStatement":
			var payload executePayload
			_ = json.NewDecoder(r.Body).Decode(&payload)
			payloadCh <- payload
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{}})
			return
		default:
			t.Fatalf("unexpected target %q", target)
		}
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()
	original := `SELECT * FROM 'log_issue' WHERE aid='xxxx' LIMIT 20`

	result, err := adapter.Execute(context.Background(), ds, original, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	payload := <-payloadCh
	if payload.Statement != `SELECT * FROM "log_issue" WHERE aid='xxxx'` {
		t.Fatalf("Statement = %q", payload.Statement)
	}
	detail := dynamodbExplainDetailMap(t, result.Detail)
	repair, ok := detail["statementRepair"].(map[string]any)
	if !ok {
		t.Fatalf("missing statementRepair detail in %#v", detail)
	}
	if repair["originalStatement"] != original {
		t.Fatalf("originalStatement = %#v", repair["originalStatement"])
	}
	if repair["repairedStatement"] != `SELECT * FROM "log_issue" WHERE aid='xxxx' LIMIT 20` {
		t.Fatalf("repairedStatement = %#v", repair["repairedStatement"])
	}
}

func TestDynamoDBAdapter_Execute_AutoRepairsSingleQuotedIndexTarget(t *testing.T) {
	type executePayload struct {
		Statement string `json:"Statement"`
	}
	payloadCh := make(chan executePayload, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch target := r.Header.Get("X-Amz-Target"); target {
		case "DynamoDB_20120810.DescribeTable":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Table": map[string]any{
					"TableName":   "log_issue",
					"TableStatus": "ACTIVE",
					"KeySchema": []map[string]any{
						{"AttributeName": "aid", "KeyType": "HASH"},
					},
				},
			})
			return
		case "DynamoDB_20120810.ExecuteStatement":
			var payload executePayload
			_ = json.NewDecoder(r.Body).Decode(&payload)
			payloadCh <- payload
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{}})
			return
		default:
			t.Fatalf("unexpected target %q", target)
		}
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	if _, err := adapter.Execute(context.Background(), ds, `SELECT * FROM 'log_issue'.'z_id-alert_time_z_id-index' WHERE z_id='yyy'`, ExecuteOptions{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	payload := <-payloadCh
	if payload.Statement != `SELECT * FROM "log_issue"."z_id-alert_time_z_id-index" WHERE z_id='yyy'` {
		t.Fatalf("Statement = %q", payload.Statement)
	}
}

func TestDynamoDBAdapter_Execute_SuggestsCompatibleGSIForBaseTableFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch target := r.Header.Get("X-Amz-Target"); target {
		case "DynamoDB_20120810.DescribeTable":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Table": map[string]any{
					"TableName":   "log_issue",
					"TableStatus": "ACTIVE",
					"KeySchema": []map[string]any{
						{"AttributeName": "aid", "KeyType": "HASH"},
						{"AttributeName": "alert_time_z_id", "KeyType": "RANGE"},
					},
					"AttributeDefinitions": []map[string]any{
						{"AttributeName": "aid", "AttributeType": "S"},
						{"AttributeName": "alert_time_z_id", "AttributeType": "S"},
						{"AttributeName": "z_id", "AttributeType": "S"},
					},
					"GlobalSecondaryIndexes": []map[string]any{
						{
							"IndexName": "z_id-alert_time_z_id-index",
							"KeySchema": []map[string]any{
								{"AttributeName": "z_id", "KeyType": "HASH"},
								{"AttributeName": "alert_time_z_id", "KeyType": "RANGE"},
							},
							"Projection": map[string]any{"ProjectionType": "ALL"},
						},
					},
				},
			})
			return
		case "DynamoDB_20120810.ExecuteStatement":
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{}})
			return
		default:
			t.Fatalf("unexpected target %q", target)
		}
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	result, err := adapter.Execute(context.Background(), ds, `SELECT * FROM "log_issue" WHERE aid='xxxx' and z_id='yyy' LIMIT 20`, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	detail := dynamodbExplainDetailMap(t, result.Detail)
	suggestion, ok := detail["indexSuggestion"].(map[string]any)
	if !ok {
		t.Fatalf("missing indexSuggestion detail in %#v", detail)
	}
	if suggestion["index"] != "z_id-alert_time_z_id-index" {
		t.Fatalf("index = %#v", suggestion["index"])
	}
	want := `SELECT * FROM "log_issue"."z_id-alert_time_z_id-index" WHERE aid='xxxx' and z_id='yyy' LIMIT 20`
	if suggestion["suggestedStatement"] != want {
		t.Fatalf("suggestedStatement = %#v, want %q", suggestion["suggestedStatement"], want)
	}
}

func TestDynamoDBAdapter_Execute_BoundedPaginationSuggestsCompatibleGSIForBaseTableFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch target := r.Header.Get("X-Amz-Target"); target {
		case "DynamoDB_20120810.DescribeTable":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Table": map[string]any{
					"TableName":   "log_issue",
					"TableStatus": "ACTIVE",
					"KeySchema": []map[string]any{
						{"AttributeName": "aid", "KeyType": "HASH"},
						{"AttributeName": "alert_time_z_id", "KeyType": "RANGE"},
					},
					"AttributeDefinitions": []map[string]any{
						{"AttributeName": "aid", "AttributeType": "S"},
						{"AttributeName": "alert_time_z_id", "AttributeType": "S"},
						{"AttributeName": "z_id", "AttributeType": "S"},
					},
					"GlobalSecondaryIndexes": []map[string]any{
						{
							"IndexName": "z_id-alert_time_z_id-index",
							"KeySchema": []map[string]any{
								{"AttributeName": "z_id", "KeyType": "HASH"},
								{"AttributeName": "alert_time_z_id", "KeyType": "RANGE"},
							},
							"Projection": map[string]any{"ProjectionType": "ALL"},
						},
					},
				},
			})
			return
		case "DynamoDB_20120810.ExecuteStatement":
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{}})
			return
		default:
			t.Fatalf("unexpected target %q", target)
		}
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	result, err := adapter.Execute(context.Background(), ds, `SELECT * FROM "log_issue" WHERE aid='xxxx' and z_id='yyy' LIMIT 20`, ExecuteOptions{
		PageSize: 5,
		Bounds: ExecuteBounds{
			MaxReturnedRows:   20,
			MaxPages:          4,
			MaxEvaluatedItems: 20,
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	detail := dynamodbExplainDetailMap(t, result.Detail)
	suggestion, ok := detail["indexSuggestion"].(map[string]any)
	if !ok {
		t.Fatalf("missing bounded indexSuggestion detail in %#v", detail)
	}
	if suggestion["index"] != "z_id-alert_time_z_id-index" {
		t.Fatalf("index = %#v", suggestion["index"])
	}
	want := `SELECT * FROM "log_issue"."z_id-alert_time_z_id-index" WHERE aid='xxxx' and z_id='yyy' LIMIT 20`
	if suggestion["suggestedStatement"] != want {
		t.Fatalf("suggestedStatement = %#v, want %q", suggestion["suggestedStatement"], want)
	}
}

func TestDynamoDBPartiQLTableName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`SELECT * FROM "orders"`, "orders"},
		{`SELECT * FROM orders`, "orders"},
		{`SELECT * FROM tenant.orders`, "tenant.orders"},
		{`SELECT * FROM tenant.prod.orders`, "tenant.prod.orders"},
		{"SELECT * FROM `orders`", "orders"},
		{`SELECT pk, sk FROM "my.table" WHERE pk = 'x'`, "my.table"},
		{`select * from Users where id = 1`, "Users"},
		{`INSERT INTO "logs" VALUE {'pk': 'x'}`, ""},
		{`UPDATE "orders" SET status = 'done' WHERE pk = 'x'`, ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := dynamodbPartiQLTableName(tt.input)
		if got != tt.want {
			t.Errorf("dynamodbPartiQLTableName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDynamoDBPartiQLTarget(t *testing.T) {
	tests := []struct {
		input     string
		wantTable string
		wantIndex string
	}{
		{`SELECT * FROM orders.by_status`, "orders.by_status", ""},
		{`SELECT * FROM tenant.prod.orders`, "tenant.prod.orders", ""},
		{`SELECT * FROM "my.table"."by.status"`, "my.table", "by.status"},
		{"SELECT * FROM `orders`.`by_status`", "orders", "by_status"},
	}
	for _, tt := range tests {
		gotTable, gotIndex := dynamodbPartiQLTarget(tt.input)
		if gotTable != tt.wantTable || gotIndex != tt.wantIndex {
			t.Errorf("dynamodbPartiQLTarget(%q) = (%q, %q), want (%q, %q)", tt.input, gotTable, gotIndex, tt.wantTable, tt.wantIndex)
		}
	}
}

func TestDynamoDBAdapter_Execute_SourceEntity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Items": []map[string]any{
				{"pk": map[string]any{"S": "1"}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	result, err := adapter.Execute(context.Background(), ds, `SELECT * FROM "orders"`, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.SourceEntity != "orders" {
		t.Fatalf("expected SourceEntity=orders, got %q", result.SourceEntity)
	}
}

func TestDynamoDBAdapter_ExplainAccessPathClassifications(t *testing.T) {
	tests := []struct {
		name               string
		statement          string
		wantClassification string
		wantTable          string
		wantIndex          string
		wantUsedIndexes    []string
		wantPartition      string
		wantSort           string
		wantFilter         string
	}{
		{
			name:               "table partition key equality is key based",
			statement:          `SELECT * FROM "orders" WHERE pk = 'ORDER#1'`,
			wantClassification: "key_based",
			wantTable:          "orders",
			wantPartition:      "pk = 'ORDER#1'",
		},
		{
			name:               "non key equality is scan like",
			statement:          `SELECT * FROM "orders" WHERE status = 'open'`,
			wantClassification: "scan_like",
			wantTable:          "orders",
			wantFilter:         "status = 'open'",
		},
		{
			name:               "limit inside literal does not truncate where clause",
			statement:          `SELECT * FROM "orders" WHERE note = 'limit reached' AND pk = 'ORDER#1' LIMIT 5`,
			wantClassification: "key_based",
			wantTable:          "orders",
			wantPartition:      "pk = 'ORDER#1'",
			wantFilter:         "note = 'limit reached'",
		},
		{
			name:               "multiline and key predicate is key based",
			statement:          "SELECT * FROM \"orders\" WHERE status = 'open'\nAND pk = 'ORDER#1'",
			wantClassification: "key_based",
			wantTable:          "orders",
			wantPartition:      "pk = 'ORDER#1'",
			wantFilter:         "status = 'open'",
		},
		{
			name:               "sort key only is scan like",
			statement:          `SELECT * FROM "orders" WHERE sk BETWEEN 'A' AND 'Z'`,
			wantClassification: "scan_like",
			wantTable:          "orders",
			wantSort:           "sk BETWEEN 'A' AND 'Z'",
		},
		{
			name:               "known index uses index metadata",
			statement:          `SELECT * FROM "orders"."by_status" WHERE status = 'open' AND createdAt >= '2026-01-01'`,
			wantClassification: "key_based",
			wantTable:          "orders",
			wantIndex:          "by_status",
			wantUsedIndexes:    []string{"by_status"},
			wantPartition:      "status = 'open'",
			wantSort:           "createdAt >= '2026-01-01'",
		},
		{
			name:               "unquoted index target uses index metadata",
			statement:          `SELECT * FROM orders.by_status WHERE status = 'open'`,
			wantClassification: "key_based",
			wantTable:          "orders",
			wantIndex:          "by_status",
			wantUsedIndexes:    []string{"by_status"},
			wantPartition:      "status = 'open'",
		},
		{
			name:               "unquoted dotted table remains table",
			statement:          `SELECT * FROM tenant.prod.orders WHERE pk = 'ORDER#1'`,
			wantClassification: "key_based",
			wantTable:          "tenant.prod.orders",
			wantPartition:      "pk = 'ORDER#1'",
		},
		{
			name:               "partition key or equality is key based",
			statement:          `SELECT * FROM "orders" WHERE pk = 'ORDER#1' OR pk = 'ORDER#2'`,
			wantClassification: "key_based",
			wantTable:          "orders",
			wantPartition:      "pk = 'ORDER#1' OR pk = 'ORDER#2'",
		},
		{
			name:               "mixed or predicate is scan like",
			statement:          `SELECT * FROM "orders" WHERE pk = 'ORDER#1' OR status = 'open'`,
			wantClassification: "scan_like",
			wantTable:          "orders",
			wantPartition:      "pk = 'ORDER#1' OR status = 'open'",
			wantFilter:         "pk = 'ORDER#1' OR status = 'open'",
		},
		{
			name:               "multiline or predicate is scan like",
			statement:          "SELECT * FROM \"orders\" WHERE pk = 'ORDER#1'\nOR status = 'open'",
			wantClassification: "scan_like",
			wantTable:          "orders",
			wantPartition:      "pk = 'ORDER#1'\nOR status = 'open'",
			wantFilter:         "pk = 'ORDER#1'\nOR status = 'open'",
		},
		{
			name:               "unknown explicit index is unknown",
			statement:          `SELECT * FROM "orders"."missing_index" WHERE status = 'open'`,
			wantClassification: "unknown",
			wantTable:          "orders",
			wantIndex:          "missing_index",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newDynamoDBExplainTestServer(t)
			ds := dynamodbDatasourceForTest(srv.URL)
			adapter := NewDynamoDBAdapter()

			result, err := adapter.Explain(context.Background(), ds, tt.statement)
			if err != nil {
				t.Fatalf("Explain: %v", err)
			}
			detail := dynamodbExplainDetailMap(t, result.Detail)
			if got := fmt.Sprint(detail["kind"]); got != "dynamodb-access-path" {
				t.Fatalf("kind = %q, want dynamodb-access-path", got)
			}
			if got := fmt.Sprint(detail["classification"]); got != tt.wantClassification {
				t.Fatalf("classification = %q, want %q; detail=%#v", got, tt.wantClassification, detail)
			}
			if got := result.UsesIndex; got != (tt.wantClassification == "key_based") {
				t.Fatalf("UsesIndex = %v, want %v", got, tt.wantClassification == "key_based")
			}
			if fmt.Sprint(result.Indexes) != fmt.Sprint(tt.wantUsedIndexes) {
				t.Fatalf("Indexes = %#v, want %#v", result.Indexes, tt.wantUsedIndexes)
			}
			if got := fmt.Sprint(detail["table"]); got != tt.wantTable {
				t.Fatalf("table = %q, want %q", got, tt.wantTable)
			}
			if got := fmt.Sprint(detail["index"]); got != tt.wantIndex {
				t.Fatalf("index = %q, want %q", got, tt.wantIndex)
			}
			if tt.wantPartition != "" {
				if got := fmt.Sprint(detail["partitionPredicate"]); got != tt.wantPartition {
					t.Fatalf("partitionPredicate = %q, want %q", got, tt.wantPartition)
				}
			}
			if tt.wantSort != "" {
				if got := fmt.Sprint(detail["sortPredicate"]); got != tt.wantSort {
					t.Fatalf("sortPredicate = %q, want %q", got, tt.wantSort)
				}
			}
			if tt.wantFilter != "" {
				filters, ok := detail["filterLikePredicates"].([]any)
				if !ok || len(filters) == 0 || fmt.Sprint(filters[0]) != tt.wantFilter {
					t.Fatalf("filterLikePredicates = %#v, want first %q", detail["filterLikePredicates"], tt.wantFilter)
				}
			}
		})
	}
}

func TestDynamoDBAdapter_Execute_BoundedPagination(t *testing.T) {
	type executePayload struct {
		Statement string  `json:"Statement"`
		Limit     *int32  `json:"Limit"`
		NextToken *string `json:"NextToken"`
	}

	requests := make([]executePayload, 0, 3)
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if target := r.Header.Get("X-Amz-Target"); target != "DynamoDB_20120810.ExecuteStatement" {
			t.Fatalf("unexpected target %q", target)
		}
		var payload executePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests = append(requests, payload)
		call++
		switch call {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Items": []map[string]any{
					{"pk": map[string]any{"S": "1"}},
				},
				"NextToken": "token-2",
			})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Items":     []map[string]any{},
				"NextToken": "token-3",
			})
		case 3:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Items": []map[string]any{
					{"pk": map[string]any{"S": "2"}},
				},
			})
		default:
			t.Fatalf("unexpected ExecuteStatement call %d", call)
		}
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	result, err := adapter.Execute(context.Background(), ds, `SELECT * FROM "orders" WHERE status = 'open'`, ExecuteOptions{
		PageSize: 2,
		Bounds: ExecuteBounds{
			MaxReturnedRows:   2,
			MaxPages:          3,
			MaxEvaluatedItems: 6,
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.RowCount != 2 || len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows across pages, got rowCount=%d rows=%d", result.RowCount, len(result.Rows))
	}
	if result.HasMore || result.NextToken != "" {
		t.Fatalf("expected final page, got hasMore=%v nextToken=%q", result.HasMore, result.NextToken)
	}
	if result.EffectivePageSize != 2 {
		t.Fatalf("effectivePageSize = %d, want 2", result.EffectivePageSize)
	}
	if result.EffectiveLimitSource != EffectiveLimitBounded {
		t.Fatalf("effectiveLimitSource = %q, want %q", result.EffectiveLimitSource, EffectiveLimitBounded)
	}
	detail := dynamodbExplainDetailMap(t, result.Detail)
	if got := fmt.Sprint(detail["kind"]); got != "dynamodb-bounded-pagination" {
		t.Fatalf("pagination kind = %q", got)
	}
	if got := intFromDetail(t, detail, "pagesFetched"); got != 3 {
		t.Fatalf("pagesFetched = %d, want 3", got)
	}
	if got := intFromDetail(t, detail, "rowsReturned"); got != 2 {
		t.Fatalf("rowsReturned = %d, want 2", got)
	}
	if got := fmt.Sprint(detail["stopReason"]); got != "no_more_pages" {
		t.Fatalf("stopReason = %q, want no_more_pages", got)
	}
	if len(requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(requests))
	}
	if requests[0].NextToken != nil {
		t.Fatalf("unexpected first next token: %#v", requests[0].NextToken)
	}
	if requests[1].NextToken == nil || *requests[1].NextToken != "token-2" {
		t.Fatalf("second request nextToken = %#v, want token-2", requests[1].NextToken)
	}
	if requests[2].NextToken == nil || *requests[2].NextToken != "token-3" {
		t.Fatalf("third request nextToken = %#v, want token-3", requests[2].NextToken)
	}
	wantLimits := []int32{2, 1, 1}
	for i, req := range requests {
		if req.Limit == nil || *req.Limit != wantLimits[i] {
			t.Fatalf("request %d Limit = %#v, want %d", i+1, req.Limit, wantLimits[i])
		}
	}
}

func TestDynamoDBAdapter_Execute_BoundedPaginationHonorsStatementLimitAcrossPages(t *testing.T) {
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if target := r.Header.Get("X-Amz-Target"); target != "DynamoDB_20120810.ExecuteStatement" {
			t.Fatalf("unexpected target %q", target)
		}
		call++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Items": []map[string]any{
				{"pk": map[string]any{"S": fmt.Sprintf("%d", call)}},
			},
			"NextToken": fmt.Sprintf("token-%d", call+1),
		})
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	result, err := adapter.Execute(context.Background(), ds, `SELECT * FROM "orders" WHERE status = 'open' LIMIT 1`, ExecuteOptions{
		PageSize: 2,
		Bounds: ExecuteBounds{
			MaxReturnedRows:   10,
			MaxPages:          3,
			MaxEvaluatedItems: 10,
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if call != 1 {
		t.Fatalf("expected one request before statement limit stopped bounded pagination, got %d", call)
	}
	if result.RowCount != 1 || len(result.Rows) != 1 {
		t.Fatalf("expected one row from LIMIT 1, got rowCount=%d rows=%d", result.RowCount, len(result.Rows))
	}
	if !result.HasMore || result.NextToken != "token-2" {
		t.Fatalf("expected resumable next token after statement limit, got hasMore=%v nextToken=%q", result.HasMore, result.NextToken)
	}
	detail := dynamodbExplainDetailMap(t, result.Detail)
	if got := intFromDetail(t, detail, "maxReturnedRows"); got != 1 {
		t.Fatalf("maxReturnedRows = %d, want 1", got)
	}
	if got := intFromDetail(t, detail, "maxEvaluatedItems"); got != 1 {
		t.Fatalf("maxEvaluatedItems = %d, want 1", got)
	}
	if got := fmt.Sprint(detail["stopReason"]); got != "returned_row_limit" {
		t.Fatalf("stopReason = %q, want returned_row_limit", got)
	}
}

func TestDynamoDBAdapter_Execute_BoundedPaginationReportsEffectiveLimit(t *testing.T) {
	var requestLimit int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if target := r.Header.Get("X-Amz-Target"); target != "DynamoDB_20120810.ExecuteStatement" {
			t.Fatalf("unexpected target %q", target)
		}
		var payload struct {
			Limit *int32 `json:"Limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Limit == nil {
			t.Fatalf("expected request Limit")
		}
		requestLimit = *payload.Limit
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Items": []map[string]any{
				{"pk": map[string]any{"S": "1"}},
			},
			"NextToken": "more",
		})
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	result, err := adapter.Execute(context.Background(), ds, `SELECT * FROM "orders" WHERE status = 'open'`, ExecuteOptions{
		PageSize: 25,
		Bounds: ExecuteBounds{
			MaxReturnedRows:   1,
			MaxPages:          5,
			MaxEvaluatedItems: 100,
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if requestLimit != 1 {
		t.Fatalf("request Limit = %d, want 1", requestLimit)
	}
	if result.EffectivePageSize != 1 {
		t.Fatalf("effectivePageSize = %d, want 1", result.EffectivePageSize)
	}
	if result.EffectiveLimitSource != EffectiveLimitBounded {
		t.Fatalf("effectiveLimitSource = %q, want %q", result.EffectiveLimitSource, EffectiveLimitBounded)
	}
	if result.RowCount != 1 || !result.HasMore || result.NextToken != "more" {
		t.Fatalf("unexpected bounded result: rowCount=%d hasMore=%v nextToken=%q", result.RowCount, result.HasMore, result.NextToken)
	}
}

func TestDynamoDBAdapter_Execute_BoundedPaginationDoesNotPreallocateHugeReturnedLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if target := r.Header.Get("X-Amz-Target"); target != "DynamoDB_20120810.ExecuteStatement" {
			t.Fatalf("unexpected target %q", target)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{}})
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	result, err := adapter.Execute(context.Background(), ds, `SELECT * FROM "orders" WHERE status = 'open'`, ExecuteOptions{
		PageSize: 1,
		Bounds: ExecuteBounds{
			MaxReturnedRows:   1 << 30,
			MaxPages:          1,
			MaxEvaluatedItems: 1,
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.RowCount != 0 || len(result.Rows) != 0 {
		t.Fatalf("expected no rows, got rowCount=%d rows=%d", result.RowCount, len(result.Rows))
	}
}

func TestDynamoDBAdapter_Execute_BoundedPaginationClampsPageSizeOnly(t *testing.T) {
	var requestLimit int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch target := r.Header.Get("X-Amz-Target"); target {
		case "DynamoDB_20120810.DescribeTable":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Table": map[string]any{
					"TableName":   "orders",
					"TableStatus": "ACTIVE",
					"KeySchema": []map[string]any{
						{"AttributeName": "pk", "KeyType": "HASH"},
					},
				},
			})
			return
		case "DynamoDB_20120810.ExecuteStatement":
			var payload struct {
				Limit *int32 `json:"Limit"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload.Limit == nil {
				t.Fatalf("expected Limit")
			}
			requestLimit = *payload.Limit
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{}})
			return
		default:
			t.Fatalf("unexpected target %q", target)
		}
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	const requestedMaxPages = 50
	const requestedMaxEvaluatedItems = 50000
	result, err := adapter.Execute(context.Background(), ds, `SELECT * FROM "orders" WHERE status = 'open'`, ExecuteOptions{
		PageSize: dynamodbMaxPageSize + 1,
		Bounds: ExecuteBounds{
			MaxReturnedRows:   10,
			MaxPages:          requestedMaxPages,
			MaxEvaluatedItems: requestedMaxEvaluatedItems,
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if requestLimit != 10 {
		t.Fatalf("request Limit = %d, want 10", requestLimit)
	}
	detail := dynamodbExplainDetailMap(t, result.Detail)
	if got := intFromDetail(t, detail, "pageSize"); got != dynamodbMaxPageSize {
		t.Fatalf("pageSize = %d, want %d", got, dynamodbMaxPageSize)
	}
	if got := intFromDetail(t, detail, "maxPages"); got != requestedMaxPages {
		t.Fatalf("maxPages = %d, want %d (no longer clamped)", got, requestedMaxPages)
	}
	if got := intFromDetail(t, detail, "maxEvaluatedItems"); got != requestedMaxEvaluatedItems {
		t.Fatalf("maxEvaluatedItems = %d, want %d (no longer clamped)", got, requestedMaxEvaluatedItems)
	}
	clamped, ok := detail["clampedLimits"].(map[string]any)
	if !ok {
		t.Fatalf("missing clampedLimits in %#v", detail)
	}
	if clamped["pageSize"] != true {
		t.Fatalf("clampedLimits[pageSize] = %#v, want true", clamped["pageSize"])
	}
	if _, present := clamped["maxPages"]; present {
		t.Fatalf("clampedLimits should not include maxPages now that the cap is lifted, got %#v", clamped)
	}
	if _, present := clamped["maxEvaluatedItems"]; present {
		t.Fatalf("clampedLimits should not include maxEvaluatedItems now that the cap is lifted, got %#v", clamped)
	}
}

func TestDynamoDBAdapter_Execute_BoundedPaginationReportsRequestedAndEffectiveLimits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if target := r.Header.Get("X-Amz-Target"); target != "DynamoDB_20120810.ExecuteStatement" {
			t.Fatalf("unexpected target %q", target)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{}})
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	result, err := adapter.Execute(context.Background(), ds, `SELECT * FROM "orders" WHERE status = 'open'`, ExecuteOptions{
		PageSize: 100,
		Bounds: ExecuteBounds{
			MaxReturnedRows:   100,
			MaxPages:          20,
			MaxEvaluatedItems: 5000,
		},
		RequestedBounds: ExecuteBounds{
			MaxReturnedRows:   100,
			MaxPages:          50,
			MaxEvaluatedItems: 10000,
		},
		ClampedLimits: map[string]bool{
			"maxPages":          true,
			"maxEvaluatedItems": true,
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	detail := dynamodbExplainDetailMap(t, result.Detail)
	requested, ok := detail["requestedLimits"].(map[string]any)
	if !ok {
		t.Fatalf("missing requestedLimits in %#v", detail)
	}
	effective, ok := detail["effectiveLimits"].(map[string]any)
	if !ok {
		t.Fatalf("missing effectiveLimits in %#v", detail)
	}
	if got := intFromDetail(t, requested, "maxPages"); got != 50 {
		t.Fatalf("requestedLimits.maxPages = %d, want 50", got)
	}
	if got := intFromDetail(t, effective, "maxPages"); got != 20 {
		t.Fatalf("effectiveLimits.maxPages = %d, want 20", got)
	}
	if got := intFromDetail(t, requested, "maxEvaluatedItems"); got != 10000 {
		t.Fatalf("requestedLimits.maxEvaluatedItems = %d, want 10000", got)
	}
	if got := intFromDetail(t, effective, "maxEvaluatedItems"); got != 5000 {
		t.Fatalf("effectiveLimits.maxEvaluatedItems = %d, want 5000", got)
	}
	clamped, ok := detail["clampedLimits"].(map[string]any)
	if !ok {
		t.Fatalf("missing clampedLimits in %#v", detail)
	}
	if clamped["maxPages"] != true || clamped["maxEvaluatedItems"] != true {
		t.Fatalf("clampedLimits = %#v, want maxPages and maxEvaluatedItems", clamped)
	}
}

func TestDynamoDBAdapter_Execute_BoundedPaginationStopsAtEvaluatedLimit(t *testing.T) {
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Items":     []map[string]any{},
			"NextToken": "still-more",
		})
	}))
	t.Cleanup(srv.Close)

	ds := dynamodbDatasourceForTest(srv.URL)
	adapter := NewDynamoDBAdapter()

	result, err := adapter.Execute(context.Background(), ds, `SELECT * FROM "orders" WHERE status = 'open'`, ExecuteOptions{
		PageSize: 2,
		Bounds: ExecuteBounds{
			MaxReturnedRows:   10,
			MaxPages:          5,
			MaxEvaluatedItems: 2,
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if call != 1 {
		t.Fatalf("expected one request before evaluated limit, got %d", call)
	}
	if !result.HasMore || result.NextToken != "still-more" {
		t.Fatalf("expected resumable next token, got hasMore=%v token=%q", result.HasMore, result.NextToken)
	}
	detail := dynamodbExplainDetailMap(t, result.Detail)
	if got := fmt.Sprint(detail["stopReason"]); got != "evaluated_item_limit" {
		t.Fatalf("stopReason = %q, want evaluated_item_limit", got)
	}
}

func newDynamoDBExplainTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if target := r.Header.Get("X-Amz-Target"); target != "DynamoDB_20120810.DescribeTable" {
			t.Fatalf("unexpected target %q", target)
		}
		var payload struct {
			TableName string `json:"TableName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.TableName == "orders.by_status" {
			w.Header().Set("X-Amzn-Errortype", "ResourceNotFoundException")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "Cannot do operations on a non-existent table"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Table": map[string]any{
				"TableName":   payload.TableName,
				"TableStatus": "ACTIVE",
				"KeySchema": []map[string]any{
					{"AttributeName": "pk", "KeyType": "HASH"},
					{"AttributeName": "sk", "KeyType": "RANGE"},
				},
				"AttributeDefinitions": []map[string]any{
					{"AttributeName": "pk", "AttributeType": "S"},
					{"AttributeName": "sk", "AttributeType": "S"},
					{"AttributeName": "status", "AttributeType": "S"},
					{"AttributeName": "createdAt", "AttributeType": "S"},
				},
				"GlobalSecondaryIndexes": []map[string]any{
					{
						"IndexName": "by_status",
						"KeySchema": []map[string]any{
							{"AttributeName": "status", "KeyType": "HASH"},
							{"AttributeName": "createdAt", "KeyType": "RANGE"},
						},
						"Projection": map[string]any{"ProjectionType": "ALL"},
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dynamodbExplainDetailMap(t *testing.T, detail any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	return out
}

func intFromDetail(t *testing.T, detail map[string]any, key string) int {
	t.Helper()
	raw, ok := detail[key]
	if !ok {
		t.Fatalf("missing detail key %q in %#v", key, detail)
	}
	switch typed := raw.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		t.Fatalf("detail[%s] has type %T", key, raw)
	}
	return 0
}

func dynamodbDatasourceForTest(endpoint string) datasource.DataSource {
	return datasource.DataSource{
		ID:   "ds_test",
		Type: datasource.TypeDynamoDB,
		Options: map[string]any{
			"region":   "us-east-1",
			"endpoint": endpoint,
			"credentials": map[string]any{
				"accessKeyId":     "test",
				"secretAccessKey": "test",
			},
		},
	}
}
