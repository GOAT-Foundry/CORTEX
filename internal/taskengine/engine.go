package taskengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GOAT-Foundry/CORTEX/internal/store"
)

// WorkerRunner executes an agent prompt for a given agentID and locale.
type WorkerRunner func(ctx context.Context, agentID, prompt, locale string) (string, error)

// Event represents a real-time progress notification from a running TaskRun.
type Event struct {
	Type     string              `json:"type"` // "plan" | "node_start" | "node_complete" | "node_error" | "artifact" | "task_complete"
	Task     *store.TaskRun      `json:"task,omitempty"`
	Node     *store.TaskNode     `json:"node,omitempty"`
	Artifact *store.TaskArtifact `json:"artifact,omitempty"`
}

// Engine orchestrates Goal decomposition into a DAG, parallel Planner/Worker execution,
// and structured Artifact & Final Report generation.
type Engine struct {
	store          *store.Store
	runner         WorkerRunner
	maxConcurrency int

	activeMu sync.Mutex
	cancels  map[string]context.CancelFunc
}

// New creates a TaskEngine backed by SQLite and the given WorkerRunner.
func New(st *store.Store, runner WorkerRunner) *Engine {
	return &Engine{
		store:          st,
		runner:         runner,
		maxConcurrency: 3,
		cancels:        map[string]context.CancelFunc{},
	}
}

// CancelTask cancels an in-flight TaskRun execution if active.
func (e *Engine) CancelTask(id string) {
	if e == nil {
		return
	}
	e.activeMu.Lock()
	cancel, ok := e.cancels[id]
	e.activeMu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
}

// NormalizeDAG sanitizes a list of TaskNodes, removing unknown/self dependencies
// and breaking any cycles so topological execution is guaranteed to terminate.
func NormalizeDAG(nodes []store.TaskNode) []store.TaskNode {
	if len(nodes) == 0 {
		return []store.TaskNode{}
	}
	out := make([]store.TaskNode, 0, len(nodes))
	seen := map[string]bool{}
	for i, n := range nodes {
		id := strings.TrimSpace(n.ID)
		if id == "" || store.ValidateIdent(id) != nil || seen[id] {
			for seq := i + 1; ; seq++ {
				cand := fmt.Sprintf("step-%d", seq)
				if !seen[cand] {
					id = cand
					break
				}
			}
		}
		seen[id] = true
		title := strings.TrimSpace(n.Title)
		if title == "" {
			title = id
		}
		prompt := strings.TrimSpace(n.Prompt)
		if prompt == "" {
			prompt = title
		}
		agentID := strings.TrimSpace(n.AgentID)
		if agentID == "" || store.ValidateIdent(agentID) != nil {
			agentID = "default"
		}
		out = append(out, store.TaskNode{
			ID:        id,
			Title:     title,
			Prompt:    prompt,
			AgentID:   agentID,
			DependsOn: n.DependsOn,
			Status:    "pending",
		})
	}

	indexByID := map[string]int{}
	for i, n := range out {
		indexByID[n.ID] = i
	}

	// Keep only valid dependencies that exist; break cycles via DFS
	for i := range out {
		cleanDeps := []string{}
		depSeen := map[string]bool{}
		for _, dep := range out[i].DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" || dep == out[i].ID || depSeen[dep] {
				continue
			}
			if _, ok := indexByID[dep]; !ok {
				continue
			}
			cleanDeps = append(cleanDeps, dep)
			depSeen[dep] = true
		}
		out[i].DependsOn = cleanDeps
	}

	// Remove any back-edges creating a cycle
	state := make([]int, len(out)) // 0=unvisited, 1=visiting, 2=visited
	var dfs func(u int)
	dfs = func(u int) {
		state[u] = 1
		filtered := make([]string, 0, len(out[u].DependsOn))
		for _, depID := range out[u].DependsOn {
			v := indexByID[depID]
			if state[v] == 1 {
				// Cycle back-edge detected -> drop this dependency edge
				continue
			}
			if state[v] == 0 {
				dfs(v)
			}
			filtered = append(filtered, depID)
		}
		out[u].DependsOn = filtered
		state[u] = 2
	}
	for i := range out {
		if state[i] == 0 {
			dfs(i)
		}
	}
	return out
}

// DefaultFallbackDAG constructs a 3-node parallel diamond DAG when the Planner LLM
// is offline or returns unstructured text.
func DefaultFallbackDAG(goal, locale string) []store.TaskNode {
	if strings.HasPrefix(strings.ToLower(locale), "ja") {
		return []store.TaskNode{
			{
				ID:        "step-1",
				Title:     "要件調査・ドメイン分析",
				Prompt:    fmt.Sprintf("目標「%s」について、背景調査・前提整理・ドメイン分析を行ってください。", goal),
				AgentID:   "researcher",
				DependsOn: []string{},
				Status:    "pending",
			},
			{
				ID:        "step-2",
				Title:     "設計・実装プラン構築",
				Prompt:    fmt.Sprintf("目標「%s」を達成するための具体的な構成案・実装プランを作成してください。", goal),
				AgentID:   "coder",
				DependsOn: []string{},
				Status:    "pending",
			},
			{
				ID:        "step-3",
				Title:     "統合検証・最終成果物まとめ",
				Prompt:    fmt.Sprintf("上流ステップの調査結果と設計案を統合し、目標「%s」の最終成果物レポートを作成してください。", goal),
				AgentID:   "default",
				DependsOn: []string{"step-1", "step-2"},
				Status:    "pending",
			},
		}
	}
	return []store.TaskNode{
		{
			ID:        "step-1",
			Title:     "Domain Research & Requirements Analysis",
			Prompt:    fmt.Sprintf("Analyze requirements, constraints, and domain context for the goal: %s", goal),
			AgentID:   "researcher",
			DependsOn: []string{},
			Status:    "pending",
		},
		{
			ID:        "step-2",
			Title:     "Architecture & Execution Blueprint",
			Prompt:    fmt.Sprintf("Design the technical structure and actionable implementation steps for the goal: %s", goal),
			AgentID:   "coder",
			DependsOn: []string{},
			Status:    "pending",
		},
		{
			ID:        "step-3",
			Title:     "Verification & Synthesis",
			Prompt:    fmt.Sprintf("Combine the upstream research and architecture outputs into a verified deliverable for: %s", goal),
			AgentID:   "default",
			DependsOn: []string{"step-1", "step-2"},
			Status:    "pending",
		},
	}
}

func parsePlannerNodes(raw string) ([]store.TaskNode, bool) {
	s := strings.TrimSpace(raw)
	start := strings.IndexByte(s, '[')
	end := strings.LastIndexByte(s, ']')
	if start < 0 || end <= start {
		return nil, false
	}
	var nodes []store.TaskNode
	if err := json.Unmarshal([]byte(s[start:end+1]), &nodes); err != nil || len(nodes) == 0 {
		return nil, false
	}
	return NormalizeDAG(nodes), true
}

// RunInput configures an autonomous TaskRun execution.
type RunInput struct {
	ID     string
	Goal   string
	Locale string
	Nodes  []store.TaskNode // Optional pre-defined DAG; if empty, Planner decomposes Goal
}

// Execute runs the full Goal Decomposition -> Parallel Worker DAG -> Artifact & Report pipeline.
func (e *Engine) Execute(ctx context.Context, in RunInput, onEvent func(Event)) (store.TaskRun, []store.TaskArtifact, error) {
	if e == nil || e.store == nil {
		return store.TaskRun{}, nil, errors.New("taskengine: store unavailable")
	}
	goal := strings.TrimSpace(in.Goal)
	if goal == "" {
		return store.TaskRun{}, nil, errors.New("taskengine: goal is required")
	}
	taskID := strings.TrimSpace(in.ID)
	if taskID == "" {
		taskID = fmt.Sprintf("task-%d", time.Now().UTC().UnixNano())
	}
	if err := store.ValidateIdent(taskID); err != nil {
		return store.TaskRun{}, nil, err
	}
	if _, err := e.store.GetTaskRun(ctx, taskID); err == nil {
		return store.TaskRun{}, nil, fmt.Errorf("taskengine: task id %q already exists", taskID)
	}
	locale := strings.TrimSpace(in.Locale)
	if locale == "" {
		locale = "en"
	}

	execCtx, cancelExec := context.WithCancel(ctx)
	e.activeMu.Lock()
	if _, exists := e.cancels[taskID]; exists {
		e.activeMu.Unlock()
		cancelExec()
		return store.TaskRun{}, nil, fmt.Errorf("taskengine: task %q is already running", taskID)
	}
	e.cancels[taskID] = cancelExec
	e.activeMu.Unlock()
	defer func() {
		cancelExec()
		e.activeMu.Lock()
		delete(e.cancels, taskID)
		e.activeMu.Unlock()
	}()

	emit := func(ev Event) {
		if onEvent != nil {
			onEvent(ev)
		}
	}

	tr, err := e.store.SaveTaskRun(execCtx, store.TaskRun{
		ID:     taskID,
		Goal:   goal,
		Locale: locale,
		Status: "planning",
		Nodes:  []store.TaskNode{},
	})
	if err != nil {
		return store.TaskRun{}, nil, err
	}

	// Step 1: Decompose Goal into DAG nodes
	var nodes []store.TaskNode
	if len(in.Nodes) > 0 {
		nodes = NormalizeDAG(in.Nodes)
	} else {
		if e.runner != nil {
			plannerPrompt := fmt.Sprintf(
				"Decompose the following Goal into 2 to 4 parallelizable subtasks as a JSON array of objects with fields: id (e.g. \"step-1\"), title, prompt, agent_id (\"researcher\", \"coder\", or \"default\"), and depends_on (array of upstream step IDs). Output ONLY valid JSON.\n\nGoal: %s",
				goal,
			)
			planCtx, cancel := context.WithTimeout(execCtx, 45*time.Second)
			rawPlan, err := e.runner(planCtx, "default", plannerPrompt, locale)
			cancel()
			if err == nil {
				if parsed, ok := parsePlannerNodes(rawPlan); ok {
					nodes = parsed
				}
			}
		}
		if len(nodes) == 0 {
			nodes = DefaultFallbackDAG(goal, locale)
		}
	}

	tr.Nodes = nodes
	tr.Status = "running"
	if ok, _ := e.store.UpdateTaskRun(execCtx, tr); !ok {
		return store.TaskRun{}, nil, errors.New("taskengine: task was deleted during planning")
	}
	trCopy := tr
	emit(Event{Type: "plan", Task: &trCopy})

	// Step 2: Execute DAG nodes in topological parallel waves
	var (
		mu        sync.Mutex
		artifacts []store.TaskArtifact
	)

	nodeByID := func(id string) *store.TaskNode {
		for i := range tr.Nodes {
			if tr.Nodes[i].ID == id {
				return &tr.Nodes[i]
			}
		}
		return nil
	}

	for {
		mu.Lock()
		var readyIndices []int
		allDone := true
		for i := range tr.Nodes {
			st := tr.Nodes[i].Status
			if st == "completed" || st == "failed" {
				continue
			}
			allDone = false
			if st != "pending" {
				continue
			}
			depsCompleted := true
			depFailed := false
			for _, depID := range tr.Nodes[i].DependsOn {
				depNode := nodeByID(depID)
				if depNode == nil || depNode.Status == "failed" {
					depFailed = true
					break
				}
				if depNode.Status != "completed" {
					depsCompleted = false
					break
				}
			}
			if depFailed {
				tr.Nodes[i].Status = "failed"
				tr.Nodes[i].Error = "prerequisite subtask failed"
				nCopy := tr.Nodes[i]
				emit(Event{Type: "node_error", Node: &nCopy})
				continue
			}
			if depsCompleted {
				readyIndices = append(readyIndices, i)
			}
		}
		if allDone {
			mu.Unlock()
			break
		}
		if len(readyIndices) == 0 {
			// Guard against any unresolvable state
			for i := range tr.Nodes {
				if tr.Nodes[i].Status == "pending" {
					tr.Nodes[i].Status = "failed"
					tr.Nodes[i].Error = "unresolvable dependency"
				}
			}
			_, _ = e.store.UpdateTaskRun(execCtx, tr)
			mu.Unlock()
			break
		}

		for _, idx := range readyIndices {
			tr.Nodes[idx].Status = "running"
			nCopy := tr.Nodes[idx]
			emit(Event{Type: "node_start", Node: &nCopy})
		}
		if ok, _ := e.store.UpdateTaskRun(execCtx, tr); !ok {
			mu.Unlock()
			return store.TaskRun{}, nil, errors.New("taskengine: task was deleted")
		}
		mu.Unlock()

		sem := make(chan struct{}, e.maxConcurrency)
		var wg sync.WaitGroup
		for _, idx := range readyIndices {
			wg.Add(1)
			sem <- struct{}{}
			go func(nodeIdx int) {
				defer wg.Done()
				defer func() { <-sem }()

				mu.Lock()
				node := tr.Nodes[nodeIdx]
				var upstreamContext strings.Builder
				for _, depID := range node.DependsOn {
					if dep := nodeByID(depID); dep != nil && dep.Output != "" {
						upstreamContext.WriteString(fmt.Sprintf("\n--- Output from %s (%s) ---\n%s\n", dep.Title, dep.ID, dep.Output))
					}
				}
				mu.Unlock()

				workerPrompt := node.Prompt
				if upstreamContext.Len() > 0 {
					workerPrompt += "\n\n<upstream_context>" + upstreamContext.String() + "</upstream_context>"
				}

				started := time.Now()
				var (
					outText string
					runErr  error
				)
				if e.runner != nil {
					wCtx, cancel := context.WithTimeout(execCtx, 2*time.Minute)
					outText, runErr = e.runner(wCtx, node.AgentID, workerPrompt, locale)
					cancel()
				} else {
					outText = fmt.Sprintf("[%s (%s)] %s", node.Title, node.AgentID, node.Prompt)
				}
				durMs := time.Since(started).Milliseconds()

				mu.Lock()
				defer mu.Unlock()
				tr.Nodes[nodeIdx].DurationMs = durMs
				if runErr != nil {
					tr.Nodes[nodeIdx].Status = "failed"
					tr.Nodes[nodeIdx].Error = runErr.Error()
					nCopy := tr.Nodes[nodeIdx]
					_, _ = e.store.UpdateTaskRun(context.Background(), tr)
					emit(Event{Type: "node_error", Node: &nCopy})
					return
				}
				tr.Nodes[nodeIdx].Status = "completed"
				tr.Nodes[nodeIdx].Output = outText
				nCopy := tr.Nodes[nodeIdx]
				if ok, _ := e.store.UpdateTaskRun(context.Background(), tr); !ok {
					return
				}
				emit(Event{Type: "node_complete", Node: &nCopy})

				art, err := e.store.SaveTaskArtifact(context.Background(), store.TaskArtifact{
					ID:      fmt.Sprintf("%s-%s", taskID, node.ID),
					TaskID:  taskID,
					NodeID:  node.ID,
					Title:   node.Title,
					Kind:    "worker_output",
					Content: outText,
				})
				if err == nil {
					artifacts = append(artifacts, art)
					artCopy := art
					emit(Event{Type: "artifact", Artifact: &artCopy})
				}
			}(idx)
		}
		wg.Wait()
	}

	// Step 3: Synthesize Final Report & Artifact
	var reportBuilder strings.Builder
	if strings.HasPrefix(strings.ToLower(locale), "ja") {
		reportBuilder.WriteString(fmt.Sprintf("# 自律タスク実行レポート: %s\n\n", goal))
	} else {
		reportBuilder.WriteString(fmt.Sprintf("# Autonomous Task Execution Report: %s\n\n", goal))
	}
	anyFailed := false
	for _, n := range tr.Nodes {
		if n.Status == "failed" {
			anyFailed = true
		}
		reportBuilder.WriteString(fmt.Sprintf("## %s (`%s` · agent: `%s`)\n\n", n.Title, n.ID, n.AgentID))
		if n.Error != "" {
			reportBuilder.WriteString(fmt.Sprintf("- **Error**: %s\n\n", n.Error))
		} else {
			reportBuilder.WriteString(n.Output + "\n\n")
		}
	}

	tr.FinalReport = strings.TrimSpace(reportBuilder.String())
	if anyFailed {
		tr.Status = "failed"
	} else {
		tr.Status = "completed"
	}
	if ok, _ := e.store.UpdateTaskRun(context.Background(), tr); !ok {
		return store.TaskRun{}, nil, errors.New("taskengine: task was deleted before completion")
	}

	reportTitle := "Final Synthesis Report"
	if strings.HasPrefix(strings.ToLower(locale), "ja") {
		reportTitle = "最終統合レポート"
	}
	finalArt, err := e.store.SaveTaskArtifact(context.Background(), store.TaskArtifact{
		ID:      fmt.Sprintf("%s-report", taskID),
		TaskID:  taskID,
		Title:   reportTitle,
		Kind:    "report",
		Content: tr.FinalReport,
	})
	if err == nil {
		artifacts = append(artifacts, finalArt)
		artCopy := finalArt
		emit(Event{Type: "artifact", Artifact: &artCopy})
	}

	if latest, err := e.store.GetTaskRun(context.Background(), taskID); err == nil {
		tr = latest
	}
	trCopy = tr
	emit(Event{Type: "task_complete", Task: &trCopy})
	return tr, artifacts, nil
}
