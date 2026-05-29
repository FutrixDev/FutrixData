package paging

import "testing"

func TestTokenRoundTrip(t *testing.T) {
	original := Token{
		Version:      1,
		DatasourceID: "ds1",
		QueryHash:    "hash",
		PageSize:     100,
		Sort:         []SortKey{{Field: "id", Desc: false}},
		StartCursor:  []any{int64(1)},
		EndCursor:    []any{int64(100)},
		Direction:    DirectionNext,
	}

	encoded, err := Encode(original)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if decoded.QueryHash != original.QueryHash || decoded.PageSize != original.PageSize {
		t.Fatalf("unexpected token: %+v", decoded)
	}
}

func TestTokenRoundTripWithLimit(t *testing.T) {
	original := Token{
		Version:      1,
		DatasourceID: "ds1",
		QueryHash:    "hash",
		PageSize:     50,
		Limit:        5000,
		Offset:       2000,
		Sort:         []SortKey{{Field: "id", Desc: false}},
		StartCursor:  []any{int64(1)},
		EndCursor:    []any{int64(50)},
		Direction:    DirectionNext,
	}

	encoded, err := Encode(original)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if decoded.Limit != original.Limit || decoded.Offset != original.Offset {
		t.Fatalf("unexpected limit token: %+v", decoded)
	}
}
