package datasourceops

import (
	"context"
	"testing"
)

func TestIsUserApproved_Unset(t *testing.T) {
	if isUserApproved(nil) {
		t.Fatal("expected nil context to report not user-approved")
	}
	if isUserApproved(context.Background()) {
		t.Fatal("expected fresh context to report not user-approved")
	}
}

func TestWithUserApproved_Propagates(t *testing.T) {
	base := context.Background()
	marked := WithUserApproved(base)

	if !isUserApproved(marked) {
		t.Fatal("WithUserApproved should set the flag on the returned context")
	}
	if isUserApproved(base) {
		t.Fatal("WithUserApproved must not mutate its parent context")
	}
}
