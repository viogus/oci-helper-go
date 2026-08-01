package handler

import (
	"strings"

	"github.com/viogus/oci-helper-go/internal/telegram"
	"testing"
)

// TestTGInstID verifies that composite "tenantID:ocid" instance IDs survive
// the colon-based callback splitting (OCIDs never contain ':').
func TestTGInstID(t *testing.T) {
	ocid := "ocid1.instance.oc1.phx.aaaaaaa123"
	composite := "12:" + ocid

	// Simulate the router split for the various callbacks that embed IDs.
	cb := "instances:detail:" + composite
	parts := strings.Split(cb, ":")
	if got := tgInstID(parts, 2); got != composite {
		t.Errorf("detail: tgInstID = %q, want %q", got, composite)
	}

	cb = "instances:action:" + composite + ":start"
	parts = strings.Split(cb, ":")
	if got := tgInstID(parts, 2); got != composite {
		t.Errorf("action: tgInstID = %q, want %q", got, composite)
	}
	if got := parts[4]; got != "start" {
		t.Errorf("action type = %q, want start", got)
	}

	cb = "instances:terminate:confirm:" + composite + ":keep"
	parts = strings.Split(cb, ":")
	if got := tgInstID(parts, 3); got != composite {
		t.Errorf("terminate: tgInstID = %q, want %q", got, composite)
	}
	if got := parts[5]; got != "keep" {
		t.Errorf("preserve flag = %q, want keep", got)
	}

	cb = "traffic:query:" + composite
	parts = strings.Split(cb, ":")
	if got := tgInstID(parts, 2); got != composite {
		t.Errorf("traffic: tgInstID = %q, want %q", got, composite)
	}

	// Bare (non-composite) values must pass through unchanged.
	parts = strings.Split("volumes:terminate:12:ocid1.blockvolume.oc1.xyz", ":")
	if got := parts[2]; got != "12" {
		t.Errorf("tenantID = %q, want 12", got)
	}
	if got := parts[3]; got != "ocid1.blockvolume.oc1.xyz" {
		t.Errorf("volID = %q", got)
	}
}

func TestParseCIDRList(t *testing.T) {
	in := "1.2.3.4/32\n\nnot-a-cidr\n10.0.0.0/8\n"
	got := parseCIDRList(in)
	if len(got) != 2 || got[0] != "1.2.3.4/32" || got[1] != "10.0.0.0/8" {
		t.Errorf("parseCIDRList = %v, want [1.2.3.4/32 10.0.0.0/8]", got)
	}
	if got := parseCIDRList(""); len(got) != 0 {
		t.Errorf("empty input parsed to %v", got)
	}
}

func TestTGModelIDs(t *testing.T) {
	// Every selectable key must map to a concrete SiliconFlow model id.
	for key, id := range tgModelIDs {
		if id == "" {
			t.Errorf("model key %q maps to empty id", key)
		}
		if tgModelDisplayName(key) == "" {
			t.Errorf("model key %q has no display name", key)
		}
	}
}

// TestTGCallbackOrdering guards the router case-ordering fixes: the generic
// "ai" and "instances:terminate" cases must NOT shadow their more specific
// sub-cases.
func TestTGCallbackOrdering(t *testing.T) {
	srv, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	bot := telegram.New("dummy-token-for-test")

	// "ai" (generic) flips the chat into AI-waiting mode...
	srv.handleTGCallback(bot, 123, 1, "cb1", "ai")
	tgAIWaitingMu.Lock()
	if !tgAIWaiting[123] {
		tgAIWaitingMu.Unlock()
		t.Fatalf("plain ai callback did not set AI-waiting state")
	}
	tgAIWaitingMu.Unlock()

	// ...but "ai:model" / "ai:model:set:qwen" must NOT (they open the model
	// menu / set the model instead of hijacking the next message).
	srv.handleTGCallback(bot, 124, 1, "cb2", "ai:model")
	tgAIWaitingMu.Lock()
	if tgAIWaiting[124] {
		tgAIWaitingMu.Unlock()
		t.Fatalf("ai:model callback wrongly set AI-waiting state (case ordering bug)")
	}
	tgAIWaitingMu.Unlock()

	srv.handleTGCallback(bot, 125, 1, "cb3", "ai:model:set:qwen")
	tgAIModelMu.Lock()
	if tgAIModels[125] != "qwen" {
		tgAIModelMu.Unlock()
		t.Fatalf("ai:model:set:qwen did not set the per-chat model (case ordering bug)")
	}
	tgAIModelMu.Unlock()

	// "instances:terminate:<id>" must go to the confirm step and
	// "instances:terminate:confirm:<id>:keep" must be routable (it reaches
	// tgTerminateDo, which answers "Instance not found" for a missing id
	// instead of falling into the generic confirm step). Both must not
	// panic or mis-split the composite id.
	srv.handleTGCallback(bot, 126, 1, "cb4", "instances:terminate:1:ocid1.instance.oc1.xyz")
	srv.handleTGCallback(bot, 127, 1, "cb5", "instances:terminate:confirm:1:ocid1.instance.oc1.xyz:keep")
}
