package taskrunner

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/siyuqian/devpilot/internal/auth"
	"github.com/siyuqian/devpilot/internal/openspec"
	"github.com/siyuqian/devpilot/internal/project"
	"github.com/siyuqian/devpilot/internal/trello"
)

func RegisterCommands(parent *cobra.Command) {
	runCmd.Flags().String("board", "", "Trello board name (required for trello source)")
	runCmd.Flags().String("source", "", "Task source: trello or github (default from .devpilot.yaml, fallback to trello)")
	runCmd.Flags().Int("interval", 300, "Poll interval in seconds")
	runCmd.Flags().Int("timeout", 30, "Per-task timeout in minutes")
	runCmd.Flags().Int("review-timeout", 10, "Code review timeout in minutes (0 to disable)")
	runCmd.Flags().Bool("once", false, "Process one card and exit")
	runCmd.Flags().Bool("dry-run", false, "Print actions without executing")
	runCmd.Flags().Bool("no-tui", false, "Disable TUI, use plain text output")
	parent.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Autonomously process tasks from a board or issue tracker",
	Long:  "Poll a task source (Trello or GitHub Issues) for ready tasks, execute their plans via the configured agent(s), and create PRs.",
	Run: func(cmd *cobra.Command, args []string) {
		boardName, _ := cmd.Flags().GetString("board")
		sourceName, _ := cmd.Flags().GetString("source")
		interval, _ := cmd.Flags().GetInt("interval")
		timeout, _ := cmd.Flags().GetInt("timeout")
		reviewTimeout, _ := cmd.Flags().GetInt("review-timeout")
		once, _ := cmd.Flags().GetBool("once")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		noTUI, _ := cmd.Flags().GetBool("no-tui")

		dir, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get working directory:", err)
			os.Exit(1)
		}

		projectCfg, _ := project.Load(dir)

		if boardName == "" {
			if projectCfg.Board != "" {
				boardName = projectCfg.Board
			}
		}

		sourceName = projectCfg.ResolveSource(sourceName)

		var source TaskSource
		var trelloClient *trello.Client
		switch sourceName {
		case "trello":
			if boardName == "" {
				fmt.Fprintln(os.Stderr, "Error: --board is required for trello source (or run: devpilot init)")
				os.Exit(1)
			}
			creds, err := auth.Load("trello")
			if err != nil {
				fmt.Fprintln(os.Stderr, "Not logged in to Trello. Run: devpilot login trello")
				os.Exit(1)
			}
			trelloClient = trello.NewClient(creds["api_key"], creds["token"])
			source = NewTrelloSource(trelloClient, boardName)
		case "github":
			source = NewGitHubSource()
		default:
			fmt.Fprintf(os.Stderr, "Unknown source %q. Must be trello or github.\n", sourceName)
			os.Exit(1)
		}

		useOpenSpec := false
		if openspec.CheckInstalled("openspec") == nil {
			if _, err := openspec.ScanChanges(dir); err == nil {
				useOpenSpec = true
			}
		}

		// Resolve agents from project config; default to Claude.
		agents := resolveAgents(projectCfg)

		// For multi-agent mode, ensure the "Claimed By" custom field exists on the board.
		claimFieldID := ""
		if len(agents) > 1 && trelloClient != nil {
			sourceInfo, err := source.Init()
			if err == nil {
				fieldID, err := trelloClient.EnsureClaimFieldExists(sourceInfo.BoardID)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to create/verify Claimed By field: %v\n", err)
				} else {
					claimFieldID = fieldID
				}
			}
		}

		cfg := Config{
			BoardName:     boardName,
			Interval:      time.Duration(interval) * time.Second,
			Timeout:       time.Duration(timeout) * time.Minute,
			ReviewTimeout: time.Duration(reviewTimeout) * time.Minute,
			Once:          once,
			DryRun:        dryRun,
			WorkDir:       dir,
			UseOpenSpec:   useOpenSpec,
			Agents:        agents,
			ClaimFieldID:  claimFieldID,
		}

		isInteractive := term.IsTerminal(int(os.Stdout.Fd()))

		if isInteractive && !noTUI {
			runWithTUI(cfg, source, boardName)
		} else {
			runPlainText(cfg, source)
		}
	},
}

// resolveAgents converts project.AgentConfig entries into taskrunner.AgentConfig.
// Defaults to [{Name: "claude"}] if none are configured.
func resolveAgents(projectCfg *project.Config) []AgentConfig {
	if len(projectCfg.Agents) == 0 {
		return []AgentConfig{{Name: "claude"}}
	}
	agents := make([]AgentConfig, len(projectCfg.Agents))
	for i, a := range projectCfg.Agents {
		agents[i] = AgentConfig{Name: a.Name, Model: a.Model}
	}
	return agents
}

func runWithTUI(cfg Config, source TaskSource, boardName string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh := make(chan Event, 100)
	handler := func(e Event) {
		eventCh <- e
	}

	agentNames := make([]string, len(cfg.Agents))
	for i, a := range cfg.Agents {
		agentNames[i] = a.Name
	}

	model := NewTUIModel(boardName, agentNames, eventCh, cancel)
	p := tea.NewProgram(model, tea.WithAltScreen())

	var runErr error
	go func() {
		var err error
		if len(cfg.Agents) > 1 {
			mr := NewMultiRunner(cfg, source, handler)
			err = mr.Run(ctx)
		} else {
			r := New(cfg, source, WithEventHandler(handler))
			err = r.Run(ctx)
		}
		if err != nil {
			runErr = err
			eventCh <- RunnerErrorEvent{Err: err}
		}
		close(eventCh)
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "TUI error:", err)
		os.Exit(1)
	}

	if runErr != nil {
		fmt.Fprintln(os.Stderr, "Runner error:", runErr)
		os.Exit(1)
	}
}

func runPlainText(cfg Config, source TaskSource) {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	handler := func(e Event) {
		switch ev := e.(type) {
		case AgentRegisteredEvent:
			logger.Printf("[agent] Registered: %s", ev.AgentName)
		case RunnerStartedEvent:
			logger.Printf("[%s] Board: %s (%s)", ev.AgentName, ev.BoardName, ev.BoardID)
			for name, id := range ev.Lists {
				logger.Printf("  List %q → %s", name, id)
			}
		case PollingEvent:
			logger.Printf("[%s] Polling for tasks...", ev.AgentName)
		case NoTasksEvent:
			logger.Printf("[%s] No tasks. Next poll in %s", ev.AgentName, ev.NextPoll)
		case CardStartedEvent:
			logger.Printf("[%s] Started: %q on branch %s", ev.AgentName, ev.CardName, ev.Branch)
		case ToolStartEvent:
			summary := toolSummary(ev.ToolName, ev.Input)
			logger.Printf("[%s] [tool] %s %s ...", ev.AgentName, ev.ToolName, summary)
		case ToolResultEvent:
			logger.Printf("[%s] [tool] %s done (%s)", ev.AgentName, ev.ToolName, formatDuration(ev.DurationMs))
		case TextOutputEvent:
			logger.Printf("[%s] %s", ev.AgentName, truncate(ev.Text, 120))
		case StatsUpdateEvent:
			if ev.Turns > 0 {
				logger.Printf("[%s] ↑%s ↓%s turns:%d", ev.AgentName, formatTokens(ev.InputTokens), formatTokens(ev.OutputTokens), ev.Turns)
			}
		case CardDoneEvent:
			logger.Printf("[%s] Done: %q (%s) PR: %s", ev.AgentName, ev.CardName, ev.Duration, ev.PRURL)
		case CardFailedEvent:
			logger.Printf("[%s] Failed: %q — %s", ev.AgentName, ev.CardName, ev.ErrMsg)
		case ClaimCollisionEvent:
			logger.Printf("[%s] [claim-collision] Card %q already claimed by %s (ours: %s:%d, actual: %s:%d)", ev.OurAgentName, ev.CardName, ev.ActualAgentName, ev.OurAgentName, ev.OurTimestamp, ev.ActualAgentName, ev.ActualTimestamp)
		case ReviewStartedEvent:
			logger.Printf("[%s] [review] Starting for %s", ev.AgentName, ev.PRURL)
		case ReviewDoneEvent:
			logger.Printf("[%s] [review] Done (exit %d)", ev.AgentName, ev.ExitCode)
		case FixStartedEvent:
			logger.Printf("[%s] [fix] Attempt %d for %s", ev.AgentName, ev.Attempt, ev.PRURL)
		case FixDoneEvent:
			logger.Printf("[%s] [fix] Done (attempt %d, exit %d)", ev.AgentName, ev.Attempt, ev.ExitCode)
		case RunnerStoppedEvent:
			logger.Printf("[%s] Runner stopped.", ev.AgentName)
		case RunnerErrorEvent:
			if ev.Err != nil {
				logger.Printf("[%s] [error] %v", ev.AgentName, ev.Err)
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt, finishing current task...")
		cancel()
	}()

	var runErr error
	if len(cfg.Agents) > 1 {
		mr := NewMultiRunner(cfg, source, handler)
		runErr = mr.Run(ctx)
	} else {
		r := New(cfg, source, WithEventHandler(handler))
		runErr = r.Run(ctx)
	}

	if runErr != nil {
		fmt.Fprintln(os.Stderr, "Runner error:", runErr)
		os.Exit(1)
	}
}

// multiRunnerTUI is the TUI variant for multi-agent mode.
// It starts N runners, all writing to one shared event channel,
// and closes the channel only after all runners finish.
func multiRunnerTUI(cfg Config, source TaskSource, boardName string, eventCh chan Event, cancel context.CancelFunc) {
	ctx, _ := context.WithCancelCause(context.Background())
	_ = ctx
	_ = cancel
	_ = boardName
	_ = eventCh

	// Use MultiRunner.Run via runWithTUI — handled inline in runWithTUI now.
}

// runMultiAgentWithTUI runs multiple agents sharing the same event channel.
// Each runner goroutine is independent; all write to eventCh.
// The channel is closed only after ALL runners finish.
func runMultiAgentWithTUI(cfg Config, source TaskSource, eventCh chan Event, ctx context.Context) error {
	agents := cfg.Agents
	var wg sync.WaitGroup
	errs := make([]error, len(agents))

	for i, ac := range agents {
		i, ac := i, ac
		wg.Add(1)
		go func() {
			defer wg.Done()
			adapter, err := NewAgentAdapter(ac)
			if err != nil {
				errs[i] = err
				return
			}
			agentCfg := cfg
			agentCfg.Agents = []AgentConfig{ac}
			r := New(agentCfg, source,
				WithEventHandler(func(e Event) { eventCh <- e }),
				WithAdapter(adapter),
			)
			errs[i] = r.Run(ctx)
		}()
	}

	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
