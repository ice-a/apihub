package notify

import (
	"encoding/json"
	"testing"
)

func TestChangeJSONRoundTrip(t *testing.T) {
	change := Change{InstanceID: "abc123", Kind: "control"}
	payload, err := json.Marshal(change)
	if err != nil {
		t.Fatalf("marshal change: %v", err)
	}

	var decoded Change
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal change: %v", err)
	}
	if decoded != change {
		t.Fatalf("round trip mismatch: got %+v want %+v", decoded, change)
	}
}

func TestNewInstanceIDIsUnique(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		id := newInstanceID()
		if id == "" {
			t.Fatal("instance id must not be empty")
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate instance id %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestStateChannelIsStable(t *testing.T) {
	if StateChannel != "gptload_state_change" {
		t.Fatalf("state channel changed: %q", StateChannel)
	}
}
