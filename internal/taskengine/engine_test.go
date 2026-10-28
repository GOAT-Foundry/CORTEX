package taskengine_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GOAT-Foundry/CORTEX/internal/store"
	"github.com/GOAT-Foundry/CORTEX/internal/taskengine"
)

func TestNormalizeDAGBreaksCycles(t *testing.T) {
	raw := []store.TaskNode{
		{ID: "step-2", Title: "A", DependsOn: []string{"step-1"}},
		{ID: "", Title: "B", DependsOn: []string{"step-2"}}, // empty ID at index 1 would collide with step-2 without collision loop
	}
	norm := taskengine.NormalizeDAG(raw)
	if len(norm) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(norm))
	}
	if norm[0].ID == norm[1].ID {
		t.Fatalf("expected unique node IDs, got %q and %q", norm[0].ID, norm[1].ID)
	}
	if len(norm[0].DependsOn) > 0 && len(norm[1].DependsOn) > 0 {
		t.Fatalf("expected cycle back-edge to be removed, got %+v", norm)
	}
}

func TestExecuteParallelDAGAndArtifacts(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	var calls int32
	eng := taskengine.New(st, func(ctx context.Context, agentID, prompt, locale string) (string, error) {
		n := atomic.AddInt32(&calls, 1)
		// First call is Planner decomposition
		if n == 1 {
			return `[
				{"id":"step-1","title":"Research","prompt":"Research topic","agent_id":"researcher","depends_on":[]},
				{"id":"step-2","title":"Code","prompt":"Write code","agent_id":"coder","depends_on":[]},
				{"id":"step-3","title":"Synthesize","prompt":"Combine results","agent_id":"default","depends_on":["step-1","step-2"]}
			]`, nil
		}
		return "Worker(" + agentID + ") completed: " + prompt, nil
	})

	var eventTypes []string
	tr, arts, err := eng.Execute(context.Background(), taskengine.RunInput{
		ID:     "task-test-1",
		Goal:   "Build domain prototype",
		Locale: "en",
	}, func(ev taskengine.Event) {
		eventTypes = append(eventTypes, ev.Type)
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if tr.Status != "completed" {
		t.Fatalf("expected completed status, got %s", tr.Status)
	}
	if len(tr.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(tr.Nodes))
	}
	// step-3 should have received upstream context from step-1 and step-2
	if !strings.Contains(tr.Nodes[2].Output, "Output from Research (step-1)") {
		t.Fatalf("expected step-3 to include upstream output from step-1, got: %s", tr.Nodes[2].Output)
	}
	// 3 worker artifacts + 1 final report artifact = 4 artifacts
	if len(arts) != 4 {
		t.Fatalf("expected 4 artifacts, got %d", len(arts))
	}
	if len(eventTypes) == 0 {
		t.Fatal("expected progress events to be emitted")
	}

	// Verify duplicate task ID is rejected
	if _, _, err := eng.Execute(context.Background(), taskengine.RunInput{
		ID:   "task-test-1",
		Goal: "Duplicate ID run",
	}, nil); err == nil {
		t.Fatal("expected error when reusing existing task ID")
	}
}
