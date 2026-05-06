package evidence

import "testing"

func TestVerifyBundle(t *testing.T) {
	result, err := VerifyBundle("../../examples/product-export")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pass {
		t.Fatalf("expected bundle to pass: %+v", result)
	}
}
