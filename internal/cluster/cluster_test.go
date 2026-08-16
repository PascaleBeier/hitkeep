package cluster

import "testing"

func TestNewManagerRequiresLogger(t *testing.T) {
	if _, err := NewManager(nil, nil); err == nil {
		t.Fatal("expected missing logger error")
	}
}
