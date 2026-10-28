package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GOAT-Foundry/CORTEX/internal/store"
)

// PromptRunner executes a scheduled prompt with a target agent persona.
type PromptRunner func(ctx context.Context, agentID, prompt string) (string, error)

// MinInterval is the minimum allowed duration for interval timers to prevent tight loops.
const MinInterval = 5 * time.Second

// ValidateSchedule validates a timer schedule ("once" + run_at, duration interval, or 5-field Cron)
// and computes the next execution time after now.
func ValidateSchedule(schedule, runAt string, now time.Time) (time.Time, error) {
	s := strings.TrimSpace(schedule)
	if s == "" {
		return time.Time{}, errors.New("scheduler: schedule is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	if strings.EqualFold(s, "once") {
		ra := strings.TrimSpace(runAt)
		if ra == "" {
			return time.Time{}, errors.New("scheduler: run_at (RFC3339) is required when schedule is 'once'")
		}
		t, err := time.Parse(time.RFC3339, ra)
		if err != nil {
			return time.Time{}, fmt.Errorf("scheduler: invalid run_at RFC3339 timestamp %q: %w", ra, err)
		}
		return t.UTC(), nil
	}

	// Check duration interval (e.g. "30s", "15m", "1h", "@every 15m")
	durCand := s
	if strings.HasPrefix(strings.ToLower(durCand), "@every ") {
		durCand = strings.TrimSpace(durCand[7:])
	}
	if d, err := time.ParseDuration(durCand); err == nil {
		if d < MinInterval {
			return time.Time{}, fmt.Errorf("scheduler: interval %s must be at least %s", d, MinInterval)
		}
		return now.Add(d), nil
	}

	// Standard 5-field Cron expression (or @hourly / @daily / @weekly)
	spec, err := parseCron(s)
	if err != nil {
		return time.Time{}, err
	}
	return spec.next(now)
}

type cronField struct {
	wildcard bool
	allowed  [60]bool
}

type cronSpec struct {
	minute cronField // 0-59
	hour   cronField // 0-23
	dom    cronField // 1-31
	month  cronField // 1-12
	dow    cronField // 0-6 (Sunday=0)
}

func parseCron(expr string) (cronSpec, error) {
	switch strings.ToLower(strings.TrimSpace(expr)) {
	case "@hourly":
		expr = "0 * * * *"
	case "@daily", "@midnight":
		expr = "0 0 * * *"
	case "@weekly":
		expr = "0 0 * * 0"
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return cronSpec{}, fmt.Errorf("scheduler: invalid schedule %q (expected duration like '15m', 'once', or 5-field cron 'm h dom mon dow')", expr)
	}
	minF, err := parseCronField(fields[0], 0, 59, false)
	if err != nil {
		return cronSpec{}, fmt.Errorf("scheduler: cron minute %q: %w", fields[0], err)
	}
	hourF, err := parseCronField(fields[1], 0, 23, false)
	if err != nil {
		return cronSpec{}, fmt.Errorf("scheduler: cron hour %q: %w", fields[1], err)
	}
	domF, err := parseCronField(fields[2], 1, 31, false)
	if err != nil {
		return cronSpec{}, fmt.Errorf("scheduler: cron day-of-month %q: %w", fields[2], err)
	}
	monF, err := parseCronField(fields[3], 1, 12, false)
	if err != nil {
		return cronSpec{}, fmt.Errorf("scheduler: cron month %q: %w", fields[3], err)
	}
	dowF, err := parseCronField(fields[4], 0, 7, true)
	if err != nil {
		return cronSpec{}, fmt.Errorf("scheduler: cron day-of-week %q: %w", fields[4], err)
	}
	return cronSpec{
		minute: minF,
		hour:   hourF,
		dom:    domF,
		month:  monF,
		dow:    dowF,
	}, nil
}

func parseCronField(part string, minVal, maxVal int, isDOW bool) (cronField, error) {
	var f cronField
	if part == "*" {
		f.wildcard = true
		for i := minVal; i <= maxVal; i++ {
			idx := i
			if isDOW && idx == 7 {
				idx = 0
			}
			f.allowed[idx] = true
		}
		return f, nil
	}

	for _, seg := range strings.Split(part, ",") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return cronField{}, errors.New("empty segment")
		}
		step := 1
		rangePart := seg
		if slash := strings.IndexByte(seg, '/'); slash >= 0 {
			sVal, err := strconv.Atoi(seg[slash+1:])
			if err != nil || sVal <= 0 {
				return cronField{}, fmt.Errorf("invalid step in %q", seg)
			}
			step = sVal
			rangePart = seg[:slash]
		}

		var start, end int
		if rangePart == "*" {
			start = minVal
			end = maxVal
		} else if dash := strings.IndexByte(rangePart, '-'); dash >= 0 {
			var err1, err2 error
			start, err1 = strconv.Atoi(rangePart[:dash])
			end, err2 = strconv.Atoi(rangePart[dash+1:])
			if err1 != nil || err2 != nil || start > end {
				return cronField{}, fmt.Errorf("invalid range %q", rangePart)
			}
		} else {
			v, err := strconv.Atoi(rangePart)
			if err != nil {
				return cronField{}, fmt.Errorf("invalid value %q", rangePart)
			}
			start = v
			if strings.Contains(seg, "/") {
				end = maxVal
			} else {
				end = v
			}
		}

		if start < minVal || end > maxVal {
			return cronField{}, fmt.Errorf("value out of bounds [%d-%d] in %q", minVal, maxVal, seg)
		}
		for i := start; i <= end; i += step {
			idx := i
			if isDOW && idx == 7 {
				idx = 0
			}
			f.allowed[idx] = true
		}
	}
	return f, nil
}

func (c cronSpec) matches(t time.Time) bool {
	if !c.minute.allowed[t.Minute()] || !c.hour.allowed[t.Hour()] || !c.month.allowed[int(t.Month())] {
		return false
	}
	domMatch := c.dom.allowed[t.Day()]
	dowMatch := c.dow.allowed[int(t.Weekday())]
	if !c.dom.wildcard && !c.dow.wildcard {
		return domMatch || dowMatch
	}
	return domMatch && dowMatch
}

func (c cronSpec) next(from time.Time) (time.Time, error) {
	cand := from.UTC().Truncate(time.Minute).Add(time.Minute)
	limit := cand.Add(8 * 366 * 24 * time.Hour)
	for !cand.After(limit) {
		if !c.month.allowed[int(cand.Month())] {
			cand = time.Date(cand.Year(), cand.Month()+1, 1, 0, 0, 0, 0, time.UTC)
			continue
		}
		domMatch := c.dom.allowed[cand.Day()]
		dowMatch := c.dow.allowed[int(cand.Weekday())]
		dayOK := domMatch && dowMatch
		if !c.dom.wildcard && !c.dow.wildcard {
			dayOK = domMatch || dowMatch
		}
		if !dayOK {
			cand = time.Date(cand.Year(), cand.Month(), cand.Day()+1, 0, 0, 0, 0, time.UTC)
			continue
		}
		if !c.hour.allowed[cand.Hour()] {
			cand = time.Date(cand.Year(), cand.Month(), cand.Day(), cand.Hour()+1, 0, 0, 0, time.UTC)
			continue
		}
		if !c.minute.allowed[cand.Minute()] {
			cand = cand.Add(time.Minute)
			continue
		}
		return cand, nil
	}
	return time.Time{}, errors.New("scheduler: no matching cron occurrence within 8 years")
}

// Engine evaluates active timers in the background and dispatches agent runs.
type Engine struct {
	store  *store.Store
	runner PromptRunner

	mu      sync.Mutex
	running map[string]bool
	sem     chan struct{}
}

// New creates a new background timer scheduler Engine.
func New(st *store.Store, runner PromptRunner) *Engine {
	return &Engine{
		store:   st,
		runner:  runner,
		running: map[string]bool{},
		sem:     make(chan struct{}, 4),
	}
}

// Start launches the background ticker loop until ctx is canceled.
func (e *Engine) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				e.tickInternal(ctx, now.UTC(), true)
			}
		}
	}()
}

// Tick evaluates all enabled timers at reference time now and executes due timers concurrently.
func (e *Engine) Tick(ctx context.Context, now time.Time) {
	e.tickInternal(ctx, now.UTC(), false)
}

func (e *Engine) tickInternal(ctx context.Context, now time.Time, async bool) {
	if e == nil || e.store == nil {
		return
	}
	now = now.UTC()
	timers, err := e.store.ListTimers(ctx)
	if err != nil {
		return
	}
	var wg sync.WaitGroup
	for _, t := range timers {
		if !t.Enabled {
			continue
		}
		if t.NextRunAt == "" {
			if next, err := ValidateSchedule(t.Schedule, t.RunAt, now); err == nil {
				_, _ = e.store.UpdateTimerRuntimeState(ctx, t.ID, t.LastRunAt, next.Format(time.RFC3339), t.LastStatus, false, true)
			}
			continue
		}
		dueAt, err := time.Parse(time.RFC3339, t.NextRunAt)
		if err != nil {
			continue
		}
		if !now.Before(dueAt) {
			timerCopy := t
			wg.Add(1)
			go func() {
				defer wg.Done()
				e.sem <- struct{}{}
				defer func() { <-e.sem }()
				_, _ = e.executeTimer(ctx, timerCopy, "schedule", now)
			}()
		}
	}
	if !async {
		wg.Wait()
	}
}

// TriggerNow runs a timer immediately by ID and records its execution log.
func (e *Engine) TriggerNow(ctx context.Context, id string) (store.TimerRunEntry, error) {
	if e == nil || e.store == nil {
		return store.TimerRunEntry{}, errors.New("scheduler: store unavailable")
	}
	t, err := e.store.GetTimer(ctx, id)
	if err != nil {
		return store.TimerRunEntry{}, err
	}
	return e.executeTimer(ctx, t, "manual", time.Now().UTC())
}

func (e *Engine) claim(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running[id] {
		return false
	}
	e.running[id] = true
	return true
}

func (e *Engine) release(id string) {
	e.mu.Lock()
	delete(e.running, id)
	e.mu.Unlock()
}

func (e *Engine) executeTimer(ctx context.Context, t store.TimerEntry, trigger string, refTime time.Time) (store.TimerRunEntry, error) {
	if !e.claim(t.ID) {
		return store.TimerRunEntry{}, fmt.Errorf("scheduler: timer %q is already running", t.ID)
	}
	defer e.release(t.ID)

	started := time.Now().UTC()
	lastRunStr := started.Format(time.RFC3339)
	nextRunStr := t.NextRunAt
	disable := false

	// Advance NextRunAt (or disable one-shot timer when triggered by schedule)
	if strings.EqualFold(strings.TrimSpace(t.Schedule), "once") {
		if trigger == "schedule" {
			disable = true
			nextRunStr = ""
		}
	} else if t.Enabled {
		if next, err := ValidateSchedule(t.Schedule, t.RunAt, refTime); err == nil {
			nextRunStr = next.Format(time.RFC3339)
		}
	}

	requireEnabled := trigger == "schedule"
	updated, err := e.store.UpdateTimerRuntimeState(ctx, t.ID, lastRunStr, nextRunStr, "running", disable, requireEnabled)
	if err != nil || !updated {
		return store.TimerRunEntry{}, errors.New("scheduler: timer was disabled or deleted before execution")
	}

	runRec, err := e.store.RecordTimerRunStart(ctx, t.ID, t.Name, trigger)
	if err != nil {
		return store.TimerRunEntry{}, err
	}

	var (
		output string
		runErr error
	)
	if e.runner != nil {
		runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		output, runErr = e.runner(runCtx, t.AgentID, t.Prompt)
		cancel()
	} else {
		output = "[No runner configured]"
	}

	durationMs := time.Since(started).Milliseconds()
	status := "succeeded"
	errMsg := ""
	if runErr != nil {
		status = "failed"
		errMsg = runErr.Error()
	}

	finished, _ := e.store.RecordTimerRunFinish(context.Background(), runRec.ID, status, output, errMsg, durationMs)
	_, _ = e.store.UpdateTimerRuntimeState(context.Background(), t.ID, lastRunStr, nextRunStr, status, disable, false)
	return finished, nil
}
