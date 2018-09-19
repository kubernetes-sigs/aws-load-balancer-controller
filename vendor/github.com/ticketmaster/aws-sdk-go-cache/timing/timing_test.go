package timing

import (
	"context"
	"testing"
)

func Test_GetData(t *testing.T) {
	d := GetData(context.Background())
	if d != nil {
		t.Errorf("GetData on empty context returned non-nil")
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, timingContextKey, &Data{})
	d = GetData(ctx)
	if d == nil {
		t.Errorf("GetData returned nil")
	}
}
