package taskrunner

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Config struct {
	BoardName      string
	Interval       time.Duration
	Timeout        time.Duration
	ReviewTimeout  time.Duration // 0 disables code review
	Once           bool
	DryRun         bool
	WorkDir        string
	UseOpenSpec    bool
	Agents         []AgentConfig // empty defaults to [{Name: "claude"}]
	ClaimFieldID   string        // Trello custom field ID for task claiming (multi-agent locking)
}

// agentName returns the single agent name for this config (used when there is exactly one).
func (c Config) agentName() string {
	if len(c.Agents) == 1 {
		return c.Agents[0].Name
	}
	return "claude"
}

type Runner struct {
	config       Config
	source       TaskSource
	executor     *Executor
	reviewer     *Reviewer
	git          *GitOps
	logger       *log.Logger
	eventHandler EventHandler
	adapter      AgentAdapter
}

// RunnerOption configures a Runner.
type RunnerOption func(*Runner)

// WithEventHandler sets an event handler that receives runner lifecycle events.
func WithEventHandler(handler EventHandler) RunnerOption {
	return func(r *Runner) {
		r.eventHandler = handler
	}
}

// WithAdapter sets a specific AgentAdapter on the runner.
func WithAdapter(adapter AgentAdapter) RunnerOption {
	return func(r *Runner) {
		r.adapter = adapter
	}
}

func New(cfg Config, source TaskSource, opts ...RunnerOption) *Runner {
	r := &Runner{
		config: cfg,
		source: source,
		git:    NewGitOps(cfg.WorkDir),
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
	for _, opt := range opts {
		opt(r)
	}

	// When event handler is set, silence the logger to avoid duplicate output.
	if r.eventHandler != nil {
		r.logger = log.New(io.Discard, "", 0)
	}

	// Default to Claude if no adapter was configured.
	if r.adapter == nil {
		agentCfg := AgentConfig{}
		if len(cfg.Agents) == 1 {
			agentCfg = cfg.Agents[0]
		}
		r.adapter = newClaudeAdapter(agentCfg)
	}

	// When event handler is set, use adapter-based streaming.
	var execOpts []ExecutorOption
	if r.eventHandler != nil {
		execOpts = append(execOpts,
			WithAgentAdapter(r.adapter),
			WithEmitHandler(r.eventHandler),
		)
	}
	r.executor = NewExecutor(execOpts...)

	if cfg.ReviewTimeout > 0 {
		r.reviewer = NewReviewer()
	}
	return r
}

func (r *Runner) agentName() string {
	if r.adapter != nil {
		return r.adapter.Name()
	}
	return "claude"
}

func (r *Runner) emit(e Event) {
	if r.eventHandler != nil {
		r.eventHandler(e)
	}
}

func (r *Runner) init() error {
	r.logger.Printf("Initializing task source...")
	info, err := r.source.Init()
	if err != nil {
		return err
	}
	r.logger.Printf("Connected: %s", info.DisplayName)
	r.emit(RunnerStartedEvent{BoardName: info.DisplayName, BoardID: info.BoardID, Lists: info.Lists, AgentName: r.agentName()})
	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	if err := r.init(); err != nil {
		return err
	}

	// Pre-flight: ensure working directory is clean
	clean, err := r.git.IsClean()
	if err != nil {
		return fmt.Errorf("check working directory: %w", err)
	}
	if !clean {
		return fmt.Errorf("working directory has uncommitted changes; commit or stash them before running")
	}

	r.logger.Println("Runner started. Polling for tasks...")

	for {
		select {
		case <-ctx.Done():
			r.logger.Println("Shutting down.")
			r.emit(RunnerStoppedEvent{AgentName: r.agentName()})
			return nil
		default:
		}

		r.emit(PollingEvent{AgentName: r.agentName()})
		tasks, err := r.source.FetchReady()
		if err != nil {
			r.logger.Printf("Error polling: %v. Retrying in %s...", err, r.config.Interval)
			r.emit(RunnerErrorEvent{Err: err, AgentName: r.agentName()})
			if !r.sleep(ctx, r.config.Interval) {
				r.logger.Println("Shutting down.")
				r.emit(RunnerStoppedEvent{AgentName: r.agentName()})
				return nil
			}
			continue
		}

		if len(tasks) == 0 {
			r.logger.Printf("No tasks. Sleeping %s...", r.config.Interval)
			r.emit(NoTasksEvent{NextPoll: r.config.Interval, AgentName: r.agentName()})
			if !r.sleep(ctx, r.config.Interval) {
				r.logger.Println("Shutting down.")
				r.emit(RunnerStoppedEvent{AgentName: r.agentName()})
				return nil
			}
			continue
		}

		SortByPriority(tasks)
		task := tasks[0]
		r.processCard(ctx, task)

		if r.config.Once {
			r.logger.Println("--once flag set. Exiting.")
			r.emit(RunnerStoppedEvent{AgentName: r.agentName()})
			return nil
		}
	}
}

func (r *Runner) processCard(ctx context.Context, task Task) {
	start := time.Now()
	agentName := r.agentName()
	r.logger.Printf("Processing card: %q (%s)", task.Name, task.ID)

	if task.Description == "" {
		r.logger.Printf("Card has empty description, marking as failed")
		r.source.MarkFailed(task.ID, "❌ Task failed\nError: Empty plan — card description is empty")
		return
	}

	if r.config.DryRun {
		r.logger.Printf("[DRY RUN] Would process card: %q", task.Name)
		return
	}

	// Move to In Progress
	if err := r.source.MarkInProgress(task.ID); err != nil {
		r.logger.Printf("Failed to move card to In Progress: %v", err)
	}

	// Claim the card for multi-agent coordination
	ourClaim := r.claimCard(task)

	// Verify we still own the card before doing irreversible operations
	if !r.verifyCardOwnership(task, ourClaim) {
		r.logger.Printf("Lost card ownership due to race condition, skipping to next card")
		return
	}

	// Git: checkout main, pull, create branch
	branch := r.git.BranchName(task.ID, task.Name)
	if err := r.git.CheckoutMain(); err != nil {
		r.failCard(task, start, fmt.Sprintf("git checkout main: %v", err))
		return
	}
	r.git.Pull() // best-effort
	if err := r.git.CreateBranch(branch); err != nil {
		r.failCard(task, start, fmt.Sprintf("git create branch: %v", err))
		return
	}
	r.emit(CardStartedEvent{CardID: task.ID, CardName: task.Name, Branch: branch, AgentName: agentName})

	// Build prompt
	prompt := r.buildPrompt(task)

	// Execute
	taskCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	result, err := r.executor.Run(taskCtx, prompt)

	// Save log
	r.saveLog(task.ID, result, ourClaim)

	if err != nil || result.ExitCode != 0 {
		errMsg := "non-zero exit code"
		if result.TimedOut {
			errMsg = "execution timed out"
		} else if result.Stderr != "" {
			errMsg = truncate(result.Stderr, 500)
		}
		r.failCard(task, start, errMsg)
		r.git.CheckoutMain()
		return
	}

	// Verify agent produced commits before pushing
	hasCommits, err := r.git.HasNewCommits(branch)
	if err != nil {
		r.failCard(task, start, fmt.Sprintf("check commits: %v", err))
		r.git.CheckoutMain()
		return
	}
	if !hasCommits {
		r.failCard(task, start, fmt.Sprintf("%s produced no commits on task branch", agentName))
		r.git.CheckoutMain()
		return
	}

	// Push and create PR
	if err := r.git.Push(branch); err != nil {
		r.failCard(task, start, fmt.Sprintf("git push: %v", err))
		r.git.CheckoutMain()
		return
	}

	prBody := fmt.Sprintf("## Task\n%s\n\n🤖 Executed by devpilot runner (%s)", task.URL, agentName)
	prURL, err := r.git.CreatePR(task.Name, prBody)
	if err != nil {
		r.failCard(task, start, fmt.Sprintf("create PR: %v", err))
		r.git.CheckoutMain()
		return
	}

	// Code review gate (blocking with self-heal loop)
	if r.reviewer != nil {
		approved := false
		for attempt := 0; attempt <= MaxReviewRetries; attempt++ {
			r.logger.Printf("Running code review for PR: %s (attempt %d)", prURL, attempt+1)
			r.emit(ReviewStartedEvent{PRURL: prURL, AgentName: agentName})
			reviewCtx, reviewCancel := context.WithTimeout(ctx, r.config.ReviewTimeout)
			reviewResult, reviewErr := r.reviewer.Review(reviewCtx, prURL)
			reviewCancel()

			if reviewErr != nil {
				r.logger.Printf("Code review error: %v", reviewErr)
				r.emit(ReviewDoneEvent{PRURL: prURL, ExitCode: -1, AgentName: agentName})
				break
			}

			r.emit(ReviewDoneEvent{PRURL: prURL, ExitCode: reviewResult.ExitCode, AgentName: agentName})

			if IsApproved(reviewResult.Stdout) {
				r.logger.Printf("Code review approved for PR: %s", prURL)
				approved = true
				break
			}

			// Review found issues — attempt fix if retries remain
			if attempt < MaxReviewRetries {
				r.logger.Printf("Review found issues, attempting fix (attempt %d/%d)", attempt+1, MaxReviewRetries)
				r.emit(FixStartedEvent{PRURL: prURL, Attempt: attempt + 1, AgentName: agentName})
				fixCtx, fixCancel := context.WithTimeout(ctx, r.config.ReviewTimeout)
				fixResult, fixErr := r.reviewer.Fix(fixCtx, prURL)
				fixCancel()

				fixExitCode := -1
				if fixErr == nil {
					fixExitCode = fixResult.ExitCode
				}
				r.emit(FixDoneEvent{PRURL: prURL, Attempt: attempt + 1, ExitCode: fixExitCode, AgentName: agentName})

				if fixErr != nil {
					r.logger.Printf("Fix attempt failed: %v", fixErr)
					continue
				}

				// Push the fix
				if err := r.git.Push(branch); err != nil {
					r.logger.Printf("Failed to push fix: %v", err)
					r.failCard(task, start, fmt.Sprintf("push fix: %v", err))
					r.git.CheckoutMain()
					return
				}
			}
		}

		if !approved {
			r.failCard(task, start, fmt.Sprintf("code review failed after %d attempts", MaxReviewRetries+1))
			r.git.CheckoutMain()
			return
		}
	}

	if err := r.git.MergePR(); err != nil {
		r.logger.Printf("Auto-merge failed (may need approval): %v", err)
	}

	// Move to Done
	duration := time.Since(start).Round(time.Second)
	r.emit(CardDoneEvent{CardID: task.ID, CardName: task.Name, PRURL: prURL, Duration: duration, AgentName: agentName})
	comment := fmt.Sprintf("✅ Task completed by devpilot runner (%s)\nDuration: %s\nPR: %s", agentName, duration, prURL)
	r.source.MarkDone(task.ID, comment)
	r.logger.Printf("Card %q completed in %s. PR: %s", task.Name, duration, prURL)

	r.git.CheckoutMain()
	r.git.Pull()
}

func (r *Runner) buildPrompt(task Task) string {
	return r.adapter.FormatPrompt(task, r.config.UseOpenSpec, r.config.WorkDir)
}

// claimCard marks a card as claimed by this agent to prevent other agents from picking it up.
// This is part of the distributed task locking mechanism for multi-agent coordination.
// Returns the claim value (format: "agent-name:timestamp-ms") if successful, empty string otherwise.
// Claim values are used for verification: before branch creation, we re-fetch the claim to ensure
// we still own the card (detecting race conditions where another agent claimed it first).
// Gracefully handles missing custom field (returns empty string, continues without locking).
func (r *Runner) claimCard(task Task) string {
	if r.config.ClaimFieldID == "" {
		return "" // Multi-agent claiming not enabled
	}
	ts, ok := r.source.(*TrelloSource)
	if !ok {
		return "" // Not a Trello source, skipping claim
	}
	// Format: agent-name:unix-timestamp-ms
	timestamp := time.Now().UnixMilli()
	claimValue := fmt.Sprintf("%s:%d", r.agentName(), timestamp)
	if err := ts.SetCardClaimValue(task.ID, r.config.ClaimFieldID, claimValue); err != nil {
		r.logger.Printf("Warning: failed to set claim on card %s: %v", task.ID, err)
		return ""
	}
	return claimValue
}

// verifyCardOwnership re-fetches the card's "Claimed By" field to detect race conditions.
// This implements the verify step of the optimistic locking pattern: claim immediately (claimCard),
// then verify ownership before irreversible operations (branch creation).
// Returns true if we still own the card (claim value matches our recorded claim).
// Returns false if collision detected (another agent's claim value on card):
//   - Emits ClaimCollisionEvent for observability
//   - Moves card back to Ready list to allow other agents to process it
//   - Logs warning but does NOT fail the task (graceful skipping to next card)
// Handles verification failures gracefully (logs warning, returns true to proceed safely).
func (r *Runner) verifyCardOwnership(task Task, ourClaim string) bool {
	if r.config.ClaimFieldID == "" || ourClaim == "" {
		return true // Claiming not enabled, proceed safely
	}
	ts, ok := r.source.(*TrelloSource)
	if !ok {
		return true // Not a Trello source, can't verify
	}

	// Re-fetch the claim value from Trello
	currentClaim, err := ts.GetCardClaimValue(task.ID, r.config.ClaimFieldID)
	if err != nil {
		r.logger.Printf("Warning: failed to verify card ownership: %v", err)
		return true // Assume we own it if verification fails (graceful degradation)
	}

	// Check if our claim still matches
	if currentClaim == ourClaim {
		return true // We still own it
	}

	// Collision detected — another agent claimed it
	r.logger.Printf("Claim collision detected on card %q: our claim %q, current claim %q", task.ID, ourClaim, currentClaim)

	// Parse claim values to emit detailed event
	parts := splitClaim(ourClaim)
	actualParts := splitClaim(currentClaim)
	r.emit(ClaimCollisionEvent{
		CardID:          task.ID,
		CardName:        task.Name,
		OurAgentName:    parts[0],
		OurTimestamp:    parts[1],
		ActualAgentName: actualParts[0],
		ActualTimestamp: actualParts[1],
	})

	// Move card back to Ready to allow other agents to claim it
	if err := ts.MoveCardToReady(task.ID); err != nil {
		r.logger.Printf("Warning: failed to move card back to Ready after collision: %v", err)
	}

	return false // We lost the card
}

// splitClaim parses a claim value "agent-name:timestamp-ms" into a [2]interface{} array.
// Format: claim values are set by claimCard as "{agent-name}:{unix-timestamp-ms}".
// Returns [agentName, timestamp] as [interface{}, interface{}] for use in events.
// Gracefully returns ["unknown", 0] if claim is empty or parsing fails.
func splitClaim(claim string) [2]interface{} {
	if claim == "" {
		return [2]interface{}{"unknown", int64(0)}
	}
	var agentName string
	var timestamp int64
	if _, err := fmt.Sscanf(claim, "%[^:]:%d", &agentName, &timestamp); err != nil {
		return [2]interface{}{"unknown", int64(0)}
	}
	return [2]interface{}{agentName, timestamp}
}

func (r *Runner) failCard(task Task, start time.Time, errMsg string) {
	duration := time.Since(start).Round(time.Second)
	r.emit(CardFailedEvent{CardID: task.ID, CardName: task.Name, ErrMsg: errMsg, Duration: duration, AgentName: r.agentName()})
	logPath := filepath.Join(r.config.WorkDir, ".devpilot", "logs", task.ID+".log")
	comment := fmt.Sprintf("❌ Task failed\nDuration: %s\nError: %s\nSee full log: %s", duration, errMsg, logPath)
	r.source.MarkFailed(task.ID, comment)
	r.logger.Printf("Card %q failed: %s", task.Name, errMsg)
}

func (r *Runner) saveLog(cardID string, result *ExecuteResult, ourClaim string) {
	if result == nil {
		return
	}
	logDir := filepath.Join(r.config.WorkDir, ".devpilot", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		r.logger.Printf("Failed to create log directory: %v", err)
		return
	}
	logPath := filepath.Join(logDir, cardID+".log")

	// Build header with metadata
	header := fmt.Sprintf("=== Execution Log ===\nAgent: %s\nTimestamp: %s\n", r.agentName(), time.Now().Format(time.RFC3339))
	if ourClaim != "" {
		parts := splitClaim(ourClaim)
		header += fmt.Sprintf("Claimed By: %s (timestamp: %d)\n", parts[0], parts[1])
	}
	header += "\n=== STDOUT ===\n"

	content := fmt.Sprintf("%s%s\n\n=== STDERR ===\n%s\n", header, result.Stdout, result.Stderr)
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		r.logger.Printf("Failed to write log file %s: %v", logPath, err)
	}
}

func (r *Runner) sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// --- MultiRunner ---

// MultiRunner runs N Runners in parallel, one per configured agent.
// All runners share the same TaskSource; Trello's "move to In Progress"
// acts as the distributed lock preventing two agents from claiming the same card.
type MultiRunner struct {
	cfg     Config
	source  TaskSource
	handler EventHandler
}

// NewMultiRunner creates a MultiRunner for the given config and task source.
func NewMultiRunner(cfg Config, source TaskSource, handler EventHandler) *MultiRunner {
	return &MultiRunner{cfg: cfg, source: source, handler: handler}
}

// Run starts one goroutine per agent and waits for all to finish.
// Agents claim cards independently; the first to mark In Progress wins.
func (m *MultiRunner) Run(ctx context.Context) error {
	agents := m.cfg.Agents
	if len(agents) == 0 {
		agents = []AgentConfig{{Name: "claude"}}
	}

	// Emit registration event for each agent.
	if m.handler != nil {
		for _, ac := range agents {
			m.handler(AgentRegisteredEvent{AgentName: ac.Name})
		}
	}

	// Validate all agents are available before starting.
	for _, ac := range agents {
		if err := checkAgentAvailable(ac.Name); err != nil {
			return err
		}
	}

	var wg sync.WaitGroup
	errs := make([]error, len(agents))

	for i, ac := range agents {
		i, ac := i, ac // capture loop vars
		wg.Add(1)
		go func() {
			defer wg.Done()
			adapter, err := NewAgentAdapter(ac)
			if err != nil {
				errs[i] = err
				return
			}
			// Each agent gets its own single-agent Config.
			agentCfg := m.cfg
			agentCfg.Agents = []AgentConfig{ac}

			r := New(agentCfg, m.source,
				WithEventHandler(m.handler),
				WithAdapter(adapter),
			)
			errs[i] = r.Run(ctx)
		}()
	}

	wg.Wait()

	// Return first non-nil error, if any.
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// checkAgentAvailable returns an error if the agent binary is not found in PATH.
func checkAgentAvailable(name string) error {
	binary := agentBinary(name)
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("agent %q not found in PATH (expected binary: %s)", name, binary)
	}
	return nil
}

// agentBinary returns the binary name for the given agent name.
func agentBinary(name string) string {
	switch name {
	case "cursor":
		return "cursor-agent"
	default:
		return name
	}
}
