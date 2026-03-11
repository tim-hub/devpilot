package taskrunner

import "time"

// EventHandler receives runner lifecycle events.
type EventHandler func(Event)

// Event is the interface all runner events implement.
type Event interface {
	eventType() string
}

type RunnerStartedEvent struct {
	BoardName string
	BoardID   string
	Lists     map[string]string // list name -> ID
	AgentName string
}

func (e RunnerStartedEvent) eventType() string { return "runner_started" }

// AgentRegisteredEvent is emitted at startup for each configured agent.
type AgentRegisteredEvent struct {
	AgentName string
}

func (e AgentRegisteredEvent) eventType() string { return "agent_registered" }

type PollingEvent struct {
	AgentName string
}

func (e PollingEvent) eventType() string { return "polling" }

type NoTasksEvent struct {
	NextPoll  time.Duration
	AgentName string
}

func (e NoTasksEvent) eventType() string { return "no_tasks" }

type CardStartedEvent struct {
	CardID    string
	CardName  string
	Branch    string
	AgentName string
}

func (e CardStartedEvent) eventType() string { return "card_started" }

type CardDoneEvent struct {
	CardID    string
	CardName  string
	PRURL     string
	Duration  time.Duration
	AgentName string
}

func (e CardDoneEvent) eventType() string { return "card_done" }

type CardFailedEvent struct {
	CardID    string
	CardName  string
	ErrMsg    string
	Duration  time.Duration
	AgentName string
}

func (e CardFailedEvent) eventType() string { return "card_failed" }

type ReviewStartedEvent struct {
	PRURL     string
	AgentName string
}

func (e ReviewStartedEvent) eventType() string { return "review_started" }

type ReviewDoneEvent struct {
	PRURL     string
	ExitCode  int
	AgentName string
}

func (e ReviewDoneEvent) eventType() string { return "review_done" }

type FixStartedEvent struct {
	PRURL     string
	Attempt   int
	AgentName string
}

func (e FixStartedEvent) eventType() string { return "fix_started" }

type FixDoneEvent struct {
	PRURL     string
	Attempt   int
	ExitCode  int
	AgentName string
}

func (e FixDoneEvent) eventType() string { return "fix_done" }

type RunnerStoppedEvent struct {
	AgentName string
}

func (e RunnerStoppedEvent) eventType() string { return "runner_stopped" }

type RunnerErrorEvent struct {
	Err       error
	AgentName string
}

func (e RunnerErrorEvent) eventType() string { return "runner_error" }

type ToolStartEvent struct {
	ToolName  string
	Input     map[string]any
	AgentName string
}

func (e ToolStartEvent) eventType() string { return "tool_start" }

type ToolResultEvent struct {
	ToolName   string
	DurationMs int
	Truncated  bool
	AgentName  string
}

func (e ToolResultEvent) eventType() string { return "tool_result" }

type TextOutputEvent struct {
	Text      string
	AgentName string
}

func (e TextOutputEvent) eventType() string { return "text_output" }

type StatsUpdateEvent struct {
	InputTokens     int
	OutputTokens    int
	CacheReadTokens int
	Turns           int
	AgentName       string
}

func (e StatsUpdateEvent) eventType() string { return "stats_update" }

type ClaimCollisionEvent struct {
	CardID          string
	CardName        string
	OurAgentName    string
	OurTimestamp    int64 // Unix milliseconds
	ActualAgentName string
	ActualTimestamp int64 // Unix milliseconds
}

func (e ClaimCollisionEvent) eventType() string { return "claim_collision" }
