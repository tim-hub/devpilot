package taskrunner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
)

type ExecuteResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
}

// OutputLine represents a single line of output from a running command.
type OutputLine struct {
	Stream string // "stdout" or "stderr"
	Text   string
}

// OutputHandler is called for each line of output during execution.
type OutputHandler func(line OutputLine)

// ClaudeEventHandler is called for each parsed stream-json event.
type ClaudeEventHandler func(event ClaudeEvent)

type Executor struct {
	command            string
	args               []string
	outputHandler      OutputHandler
	claudeEventHandler ClaudeEventHandler

	// Adapter-based path (used when adapter is set; takes precedence over command/args).
	adapter AgentAdapter
	emit    EventHandler
}

type ExecutorOption func(*Executor)

func WithCommand(command string, args ...string) ExecutorOption {
	return func(e *Executor) {
		e.command = command
		e.args = args
	}
}

func WithOutputHandler(handler OutputHandler) ExecutorOption {
	return func(e *Executor) {
		e.outputHandler = handler
	}
}

func WithClaudeEventHandler(handler ClaudeEventHandler) ExecutorOption {
	return func(e *Executor) {
		e.claudeEventHandler = handler
	}
}

// WithAgentAdapter sets the AgentAdapter used for command building and output parsing.
// When set, adapter.BuildCommand(prompt) overrides command/args, and adapter.HandleLine
// replaces the claudeEventHandler path.
func WithAgentAdapter(adapter AgentAdapter) ExecutorOption {
	return func(e *Executor) {
		e.adapter = adapter
	}
}

// WithEmitHandler sets the EventHandler that receives runner events from the adapter.
func WithEmitHandler(emit EventHandler) ExecutorOption {
	return func(e *Executor) {
		e.emit = emit
	}
}

func NewExecutor(opts ...ExecutorOption) *Executor {
	e := &Executor{
		command: "claude",
		args:    []string{"-p", "--verbose", "--output-format", "stream-json", "--allowedTools=*"},
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func (e *Executor) Run(ctx context.Context, prompt string) (*ExecuteResult, error) {
	var cmdName string
	var args []string

	if e.adapter != nil {
		// Adapter-based path: adapter provides the full command and args.
		cmdName, args = e.adapter.BuildCommand(prompt)
	} else {
		// Legacy path: use stored command/args, appending prompt for claude.
		cmdName = e.command
		args = make([]string, len(e.args))
		copy(args, e.args)
		if e.command == "claude" {
			args = append(args, prompt)
		}
	}

	cmd := exec.CommandContext(ctx, cmdName, args...)

	if e.outputHandler == nil && e.claudeEventHandler == nil && (e.adapter == nil || e.emit == nil) {
		return e.runBuffered(ctx, cmd)
	}
	return e.runStreaming(ctx, cmd)
}

// runBuffered is the original behavior: capture all output at once.
func (e *Executor) runBuffered(ctx context.Context, cmd *exec.Cmd) (*ExecuteResult, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &ExecuteResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	return e.handleResult(ctx, err, result)
}

// runStreaming reads stdout/stderr line-by-line, calling the handler for each.
func (e *Executor) runStreaming(ctx context.Context, cmd *exec.Cmd) (*ExecuteResult, error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	// Use a process group so we can kill all child processes on cancellation.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	// Kill the entire process group when the context is cancelled.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		case <-done:
		}
	}()

	// Wrap the emit handler to inject AgentName into all bridge-emitted events.
	// Bridges (opencode, gemini, cursor) don't know the agent name themselves,
	// so the executor injects it here at the boundary where the context is known.
	var adaptedEmit EventHandler
	if e.adapter != nil && e.emit != nil {
		adaptedEmit = withAgentName(e.adapter.Name(), e.emit)
	}

	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		e.scanStream(stdoutPipe, "stdout", &stdout, adaptedEmit)
	}()
	go func() {
		defer wg.Done()
		e.scanStream(stderrPipe, "stderr", &stderr, adaptedEmit)
	}()

	wg.Wait()
	close(done)

	waitErr := cmd.Wait()

	result := &ExecuteResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	return e.handleResult(ctx, waitErr, result)
}

// withAgentName wraps an EventHandler to inject AgentName into streaming events
// (TextOutputEvent, ToolStartEvent, ToolResultEvent, StatsUpdateEvent).
// This is necessary because agent bridges emit events without knowing the agent name —
// that context only exists at the executor level.
func withAgentName(agentName string, emit EventHandler) EventHandler {
	if agentName == "" {
		return emit
	}
	return func(e Event) {
		switch ev := e.(type) {
		case TextOutputEvent:
			ev.AgentName = agentName
			emit(ev)
		case ToolStartEvent:
			ev.AgentName = agentName
			emit(ev)
		case ToolResultEvent:
			ev.AgentName = agentName
			emit(ev)
		case StatsUpdateEvent:
			ev.AgentName = agentName
			emit(ev)
		default:
			emit(e)
		}
	}
}

// scanStream reads lines from a pipe, calls the handler, and accumulates output.
// adaptedEmit is the pre-wrapped EventHandler with AgentName injected (or nil if unused).
func (e *Executor) scanStream(pipe io.Reader, stream string, buf *bytes.Buffer, adaptedEmit EventHandler) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line)
		buf.WriteByte('\n')
		if e.outputHandler != nil {
			e.outputHandler(OutputLine{Stream: stream, Text: line})
		}
		if adaptedEmit != nil && stream == "stdout" {
			e.adapter.HandleLine(line, adaptedEmit)
		} else if e.claudeEventHandler != nil && stream == "stdout" {
			event, err := ParseLine([]byte(line))
			if err == nil && event != nil {
				e.claudeEventHandler(event)
			}
		}
	}
}

// handleResult processes the command's exit status and context errors.
func (e *Executor) handleResult(ctx context.Context, err error, result *ExecuteResult) (*ExecuteResult, error) {
	if ctx.Err() != nil {
		result.TimedOut = ctx.Err() == context.DeadlineExceeded
		return result, fmt.Errorf("execution interrupted: %w", ctx.Err())
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.Sys().(syscall.WaitStatus).ExitStatus()
			return result, nil
		}
		return result, fmt.Errorf("exec failed: %w", err)
	}

	result.ExitCode = 0
	return result, nil
}
