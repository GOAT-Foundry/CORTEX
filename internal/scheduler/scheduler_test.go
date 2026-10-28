package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/GOAT-Foundry/CORTEX/internal/scheduler"
	"github.com/GOAT-Foundry/CORTEX/internal/store"
)

func TestValidateScheduleCronIntervalAndOnce(t *testing.T) {
	base := time.Date(2026, 10, 28, 10, 15, 0, 0, time.UTC)

	// 1. Duration interval
	nextDur, err := scheduler.ValidateSchedule("15m", "", base)
	if err != nil {
		t.Fatalf("interval 15m failed: %v", err)
	}
	if !nextDur.Equal(base.Add(15 * time.Minute)) {
		t.Fatalf("expected %s, got %s", base.Add(15*time.Minute), nextDur)
	}

	// Reject too-short interval
	if _, err := scheduler.ValidateSchedule("1s", "", base); err == nil {
		t.Fatal("expected error for 1s interval below MinInterval")
	}

	// 2. 5-field Cron: every hour at minute 30
	nextCron, err := scheduler.ValidateSchedule("30 * * * *", "", base)
	if err != nil {
		t.Fatalf("cron '30 * * * *' failed: %v", err)
	}
	wantCron := time.Date(2026, 10, 28, 10, 30, 0, 0, time.UTC)
	if !nextCron.Equal(wantCron) {
		t.Fatalf("expected %s, got %s", wantCron, nextCron)
	}

	// 3. One-shot RFC3339
	runAt := "2026-10-29T00:00:00Z"
	nextOnce, err := scheduler.ValidateSchedule("once", runAt, base)
	if err != nil {
		t.Fatalf("once schedule failed: %v", err)
	}
	if nextOnce.Format(time.RFC3339) != runAt {
		t.Fatalf("expected %s, got %s", runAt, nextOnce.Format(time.RFC3339))
	}

	// 4. Leap-day Cron (Feb 29) from Oct 2026 -> should find Feb 29, 2028
	nextLeap, err := scheduler.ValidateSchedule("0 0 29 2 *", "", base)
	if err != nil {
		t.Fatalf("leap-day cron failed: %v", err)
	}
	wantLeap := time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)
	if !nextLeap.Equal(wantLeap) {
		t.Fatalf("expected %s, got %s", wantLeap, nextLeap)
	}
}

func TestEngineTickAndOneShotAutoDisable(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	runCount := 0
	eng := scheduler.New(st, func(ctx context.Context, agentID, prompt string) (string, error) {
		runCount++
		return "Executed: " + prompt + " by " + agentID, nil
	})

	dueTime := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	_, err = st.SaveTimer(ctx, store.TimerEntry{
		ID:        "timer-once-1",
		Name:      "One-shot Report",
		Schedule:  "once",
		RunAt:     dueTime.Format(time.RFC3339),
		NextRunAt: dueTime.Format(time.RFC3339),
		Prompt:    "Generate daily summary",
		AgentID:   "researcher",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("save timer: %v", err)
	}

	// Tick before due time -> should not run
	eng.Tick(ctx, dueTime.Add(-1*time.Minute))
	if runCount != 0 {
		t.Fatalf("expected 0 runs before due time, got %d", runCount)
	}

	// Tick at due time -> should execute and auto-disable one-shot timer
	eng.Tick(ctx, dueTime)
	if runCount != 1 {
		t.Fatalf("expected 1 run at due time, got %d", runCount)
	}

	updated, err := st.GetTimer(ctx, "timer-once-1")
	if err != nil {
		t.Fatalf("get timer: %v", err)
	}
	if updated.Enabled {
		t.Fatal("expected one-shot timer to auto-disable after execution")
	}
	if updated.LastStatus != "succeeded" {
		t.Fatalf("expected LastStatus=succeeded, got %s", updated.LastStatus)
	}

	runs, err := st.ListTimerRuns(ctx, "timer-once-1", 10)
	if err != nil || len(runs) != 1 {
		t.Fatalf("expected 1 run log, got %d (%v)", len(runs), err)
	}
	if runs[0].Output != "Executed: Generate daily summary by researcher" {
		t.Fatalf("unexpected run output: %q", runs[0].Output)
	}
}
