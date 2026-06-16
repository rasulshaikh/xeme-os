// Package workflows is the Xeme Workflows Engine — a DAG-based
// automation runner that composes any of the Xeme engines (signal,
// enrich, ledger, ai, intel) into reusable, multi-step workflows.
//
// A workflow is a JSON document:
//
//   {
//     "name": "Daily CMO prospect",
//     "nodes": [
//       {"id": "scrape", "type": "signal.scrape", "params": {"url": "..."}},
//       {"id": "enrich", "type": "enrich.waterfall", "depends_on": ["scrape"], "params": {"min_score": 60}},
//       {"id": "score",  "type": "score.run",       "depends_on": ["enrich"]},
//       {"id": "sync",   "type": "ledger.sync",     "depends_on": ["score"]}
//     ]
//   }
//
// The executor performs a topological sort by depends_on, then runs
// independent nodes in parallel (one goroutine per node). Each node
// is retried with exponential backoff on failure. State is persisted
// to SQLite so runs can be inspected and resumed.
package workflows

import (
	"context"
	"database/sql"
	"os"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Workflow is the parsed workflow definition.
type Workflow struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Nodes       []Node                 `json:"nodes"`
	CreatedAt   time.Time              `json:"created_at,omitempty"`
}

// Node is a single step in the workflow DAG.
type Node struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`     // signal.scrape, enrich.waterfall, score.run, ledger.sync, ai.personalize, intel.boost, shell
	Params    map[string]interface{} `json:"params"`
	DependsOn []string               `json:"depends_on,omitempty"`
	Retry     int                    `json:"retry,omitempty"`     // number of retries (default 3)
	Timeout   string                 `json:"timeout,omitempty"`  // e.g. "30s"
}

// Run is one execution of a workflow.
type Run struct {
	ID          string                 `json:"id"`
	WorkflowID  string                 `json:"workflow_id"`
	Status      string                 `json:"status"`  // pending, running, completed, failed
	StartedAt   time.Time              `json:"started_at"`
	FinishedAt  *time.Time             `json:"finished_at,omitempty"`
	NodeStates  map[string]*NodeState  `json:"node_states"`
	Duration    string                 `json:"duration,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

// NodeState is the state of a single node in a run.
type NodeState struct {
	Status     string                 `json:"status"`  // pending, running, completed, failed, skipped
	StartedAt  *time.Time             `json:"started_at,omitempty"`
	FinishedAt *time.Time             `json:"finished_at,omitempty"`
	Result     map[string]interface{} `json:"result,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Attempts   int                    `json:"attempts"`
}

// Engine is the workflow executor.
type Engine struct {
	DB        *sql.DB
	mu        sync.Mutex
	Handlers  map[string]NodeHandler
}

// NodeHandler executes a node and returns the result.
type NodeHandler func(ctx context.Context, n Node) (map[string]interface{}, error)

// Open creates a workflow engine with SQLite persistence at the given path.
func Open(path string) (*Engine, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	e := &Engine{DB: db, Handlers: make(map[string]NodeHandler)}
	if err := e.migrate(); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Engine) migrate() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS workflows (
			id TEXT PRIMARY KEY,
			name TEXT,
			description TEXT,
			nodes TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS workflow_runs (
			id TEXT PRIMARY KEY,
			workflow_id TEXT,
			status TEXT,
			started_at TIMESTAMP,
			finished_at TIMESTAMP,
			node_states TEXT,
			error TEXT
		)`,
	} {
		if _, err := e.DB.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// RegisterHandler associates a node type with an executor function.
func (e *Engine) RegisterHandler(nodeType string, h NodeHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Handlers[nodeType] = h
}

// Save persists a workflow definition.
func (e *Engine) Save(w Workflow) error {
	if w.ID == "" {
		w.ID = fmt.Sprintf("wf-%d", time.Now().UnixNano())
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now()
	}
	nodes, _ := json.Marshal(w.Nodes)
	_, err := e.DB.Exec(`INSERT OR REPLACE INTO workflows (id, name, description, nodes, created_at) VALUES (?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Description, string(nodes), w.CreatedAt)
	return err
}

// Get returns a workflow by ID.
func (e *Engine) Get(id string) (*Workflow, error) {
	row := e.DB.QueryRow(`SELECT id, name, description, nodes, created_at FROM workflows WHERE id = ?`, id)
	var w Workflow
	var nodesJSON string
	if err := row.Scan(&w.ID, &w.Name, &w.Description, &nodesJSON, &w.CreatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(nodesJSON), &w.Nodes)
	return &w, nil
}

// List returns all workflows.
func (e *Engine) List() ([]Workflow, error) {
	rows, err := e.DB.Query(`SELECT id, name, description, nodes, created_at FROM workflows ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Workflow
	for rows.Next() {
		var w Workflow
		var nodesJSON string
		if err := rows.Scan(&w.ID, &w.Name, &w.Description, &nodesJSON, &w.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(nodesJSON), &w.Nodes)
		out = append(out, w)
	}
	return out, nil
}

// Run starts a workflow execution. Returns the run ID; the run continues
// asynchronously. Use GetRun to poll status.
func (e *Engine) Run(w Workflow) (*Run, error) {
	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	now := time.Now()
	run := &Run{
		ID:         runID,
		WorkflowID: w.ID,
		Status:     "running",
		StartedAt:  now,
		NodeStates: make(map[string]*NodeState),
	}
	for _, n := range w.Nodes {
		run.NodeStates[n.ID] = &NodeState{Status: "pending"}
	}
	if err := e.saveRun(run); err != nil {
		return nil, err
	}
	// Start execution in a goroutine
	go e.execute(run, w)
	return run, nil
}

// GetRun fetches a run by ID.
func (e *Engine) GetRun(id string) (*Run, error) {
	row := e.DB.QueryRow(`SELECT id, workflow_id, status, started_at, finished_at, node_states, error FROM workflow_runs WHERE id = ?`, id)
	var r Run
	var finishedAt *time.Time
	var nodeStatesJSON, errMsg sql.NullString
	if err := row.Scan(&r.ID, &r.WorkflowID, &r.Status, &r.StartedAt, &finishedAt, &nodeStatesJSON, &errMsg); err != nil {
		return nil, err
	}
	r.FinishedAt = finishedAt
	if errMsg.Valid {
		r.Error = errMsg.String
	}
	if nodeStatesJSON.Valid {
		_ = json.Unmarshal([]byte(nodeStatesJSON.String), &r.NodeStates)
	}
	if r.StartedAt.Unix() > 0 {
		dur := time.Since(r.StartedAt)
		r.Duration = dur.Truncate(time.Second).String()
	}
	return &r, nil
}

func (e *Engine) saveRun(r *Run) error {
	ns, _ := json.Marshal(r.NodeStates)
	var finishedAt *time.Time
	if r.FinishedAt != nil {
		finishedAt = r.FinishedAt
	}
	_, err := e.DB.Exec(`INSERT OR REPLACE INTO workflow_runs (id, workflow_id, status, started_at, finished_at, node_states, error) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.WorkflowID, r.Status, r.StartedAt, finishedAt, string(ns), r.Error)
	return err
}

// execute runs the workflow DAG.
func (e *Engine) execute(run *Run, w Workflow) {
	// Topological sort
	levels, err := topoSort(w.Nodes)
	if err != nil {
		run.Status = "failed"
		run.Error = "topological sort: " + err.Error()
		now := time.Now()
		run.FinishedAt = &now
		_ = e.saveRun(run)
		return
	}

	// Execute level by level, parallel within a level
	for _, level := range levels {
		var wg sync.WaitGroup
		results := make(chan nodeResult, len(level))
		for _, nodeIdx := range level {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				n := w.Nodes[idx]
				ns := run.NodeStates[n.ID]
				ns.Status = "running"
				now := time.Now()
				ns.StartedAt = &now
				ns.Attempts++
				_ = e.saveRun(run)

				h, ok := e.Handlers[n.Type]
				if !ok {
					// Unknown node type — log and skip
					ns.Status = "skipped"
					ns.Error = "no handler for type: " + n.Type
					now := time.Now()
					ns.FinishedAt = &now
					_ = e.saveRun(run)
					results <- nodeResult{idx: idx, err: nil}
					return
				}

				maxAttempts := n.Retry
				if maxAttempts == 0 {
					maxAttempts = 3
				}
				var lastErr error
				for attempt := 1; attempt <= maxAttempts; attempt++ {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
					result, err := h(ctx, n)
					cancel()
					if err == nil {
						ns.Status = "completed"
						ns.Result = result
						now := time.Now()
						ns.FinishedAt = &now
						_ = e.saveRun(run)
						results <- nodeResult{idx: idx, err: nil}
						return
					}
					lastErr = err
					if attempt < maxAttempts {
						backoff := time.Duration(1<<attempt) * time.Second
						time.Sleep(backoff)
					}
				}
				ns.Status = "failed"
				ns.Error = lastErr.Error()
				now = time.Now()
				ns.FinishedAt = &now
				_ = e.saveRun(run)
				results <- nodeResult{idx: idx, err: lastErr}
			}(nodeIdx)
		}
		wg.Wait()
		close(results)
		// If any node in this level failed, abort
		hadError := false
		for r := range results {
			if r.err != nil {
				hadError = true
			}
			_ = r
		}
		if hadError {
			run.Status = "failed"
			run.Error = "one or more nodes failed at level " + fmt.Sprintf("%d", 0)
			now := time.Now()
			run.FinishedAt = &now
			_ = e.saveRun(run)
			return
		}
	}
	run.Status = "completed"
	now := time.Now()
	run.FinishedAt = &now
	_ = e.saveRun(run)
}

type nodeResult struct {
	idx int
	err error
}

// topoSort returns nodes grouped by execution level. Level 0 has no deps.
func topoSort(nodes []Node) ([][]int, error) {
	inDeg := make(map[string]int, len(nodes))
	idxByID := make(map[string]int, len(nodes))
	for i, n := range nodes {
		idxByID[n.ID] = i
		inDeg[n.ID] = 0
	}
	for _, n := range nodes {
		for _, dep := range n.DependsOn {
			if _, ok := idxByID[dep]; !ok {
				return nil, fmt.Errorf("node %q depends on missing node %q", n.ID, dep)
			}
			inDeg[n.ID]++
		}
	}
	var levels [][]int
	for len(inDeg) > 0 {
		var level []int
		for id, d := range inDeg {
			if d == 0 {
				level = append(level, idxByID[id])
			}
		}
		if len(level) == 0 {
			return nil, fmt.Errorf("cycle detected")
		}
		levels = append(levels, level)
		// Mark these as resolved
		for _, idx := range level {
			id := nodes[idx].ID
			delete(inDeg, id)
			for _, n := range nodes {
				for _, dep := range n.DependsOn {
					if dep == id {
						inDeg[n.ID]--
					}
				}
			}
		}
	}
	return levels, nil
}

// LoadWorkflowFromFile reads a workflow definition from disk.
func LoadWorkflowFromFile(path string) (*Workflow, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	var w Workflow
	if err := json.Unmarshal([]byte(data), &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func readFile(path string) (string, error) {
	data, err := readFileBytes(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Close releases the underlying DB.
func (e *Engine) Close() error { return e.DB.Close() }

// RunSync starts a workflow and blocks until it completes.
func (e *Engine) RunSync(w Workflow) (*Run, error) {
	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	now := time.Now()
	run := &Run{
		ID:         runID,
		WorkflowID: w.ID,
		Status:     "running",
		StartedAt:  now,
		NodeStates: make(map[string]*NodeState),
	}
	for _, n := range w.Nodes {
		run.NodeStates[n.ID] = &NodeState{Status: "pending"}
	}
	if err := e.saveRun(run); err != nil {
		return nil, err
	}
	e.execute(run, w)
	return run, nil
}
