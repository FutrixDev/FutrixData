package console

import "testing"

func TestPageWindowLimitMetadata_StatementLimitCapsPageSize(t *testing.T) {
	effective, source := pageWindowLimitMetadata(200, 20, 0, EffectiveLimitDefault)
	if effective != 20 {
		t.Fatalf("effective = %d; want 20", effective)
	}
	if source != EffectiveLimitStatement {
		t.Fatalf("source = %q; want %q", source, EffectiveLimitStatement)
	}
}

func TestPageWindowLimitMetadata_UsesPageSourceWhenSmaller(t *testing.T) {
	effective, source := pageWindowLimitMetadata(20, 200, 0, EffectiveLimitPageSize)
	if effective != 20 {
		t.Fatalf("effective = %d; want 20", effective)
	}
	if source != EffectiveLimitPageSize {
		t.Fatalf("source = %q; want %q", source, EffectiveLimitPageSize)
	}
}

func TestPageWindowLimitMetadata_StatementRemainingCapsTokenPage(t *testing.T) {
	effective, source := pageWindowLimitMetadata(50, 120, 100, EffectiveLimitPagingToken)
	if effective != 20 {
		t.Fatalf("effective = %d; want 20", effective)
	}
	if source != EffectiveLimitStatement {
		t.Fatalf("source = %q; want %q", source, EffectiveLimitStatement)
	}
}
