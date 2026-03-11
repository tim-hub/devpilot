# DevPilot

[![Test](https://github.com/siyuqian/devpilot/actions/workflows/test.yml/badge.svg)](https://github.com/siyuqian/devpilot/actions/workflows/test.yml)
[![codecov](https://codecov.io/gh/siyuqian/devpilot/branch/main/graph/badge.svg)](https://codecov.io/gh/siyuqian/devpilot)
[![GitHub Downloads](https://img.shields.io/github/downloads/siyuqian/devpilot/total)](https://github.com/siyuqian/devpilot/releases)

**Autonomous development workflow automation for [Claude Code](https://claude.ai/code) and other AI agents.** Write a plan in markdown, push it to Trello, and let DevPilot execute it using one or more AI agents in parallel — creating branches, writing code, opening PRs, running code review, and auto-merging.

## How It Works

```
Plan (markdown) → devpilot push → Trello card → devpilot run → AI Agent(s) → Branch + PR
```

1. **Write a plan** — A markdown file with a `# Title` and implementation steps
2. **Push to Trello** — `devpilot push plan.md --board "My Board"` creates a card in the "Ready" list
3. **Runner picks it up** — `devpilot run` polls the board with one or more agents (configured in `.devpilot.yaml`)
4. **Agents execute in parallel** — Each agent claims different cards and runs the plan (Trello's "move to In Progress" is the distributed lock)
5. **Watch it work** — A real-time TUI dashboard adapts to show 1, 2, or 3+ agent panes with per-agent stats and output
6. **Ship it** — Branches created, code written, PRs opened, AI code review, auto-merge

## Features

- **Autonomous task execution** — Cards flow through Ready → In Progress → Done/Failed without human intervention
- **Priority scheduling** — P0/P1/P2 labels control execution order
- **Real-time TUI dashboard** — Bubble Tea terminal UI with tool call history, file tracking, token stats, and scrollable output
- **Automated code review** — A second `claude -p` invocation reviews the diff against the original plan before merging
- **OpenSpec integration** — Sync spec-driven changes to Trello or GitHub Issues with `devpilot sync`
- **Gmail AI digest** — `devpilot gmail summary` summarizes all unread emails via Claude (dry run by default, marks as read when sending to Slack)
- **Slack integration** — Send messages to channels or DMs, used as an output target for Gmail summaries
- **Built-in Claude Code skills** — PM research, Trello management, task refinement, Confluence review, and more
- **Project scaffolding** — `devpilot init` detects your stack and generates config, hooks, and skills

## Getting Started

### Prerequisites

**Core:**
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) installed and authenticated
- [GitHub CLI (`gh`)](https://cli.github.com/) installed and authenticated
- A [Trello](https://trello.com/) account with an [API key and token](https://trello.com/power-ups/admin)
- Git repository initialized in your project

**Optional:**
- [Gemini CLI](https://ai.google.dev/gemini-cli/) for multi-agent execution
- [Opencode](https://github.com/google/opencode) for multi-agent execution
- [Cursor Agent CLI](https://www.cursor.com/) for multi-agent execution
- Google OAuth credentials for Gmail integration
- Slack OAuth credentials for Slack integration

### Installation

**From release (recommended):**

```bash
curl -sSL https://raw.githubusercontent.com/siyuqian/devpilot/main/install.sh | sh
```

Optionally specify a version or directory:

```bash
curl -sSL https://raw.githubusercontent.com/siyuqian/devpilot/main/install.sh | sh -s -- --version v0.1.0 --dir ~/.local/bin
```

**From source** (requires Go 1.25+):

```bash
git clone https://github.com/siyuqian/devpilot.git
cd devpilot
make build
sudo mv bin/devpilot /usr/local/bin/
```

Verify: `devpilot --version`

### Quick Start

```bash
# 1. Initialize your project
cd your-project
devpilot init          # interactive wizard; use -y for defaults

# 2. Authenticate with Trello
devpilot login trello

# 3. Push a plan
devpilot push docs/plans/my-feature-plan.md --board "Sprint Board"

# 4. Run the task runner
devpilot run --board "Sprint Board"
```

## CLI Reference

| Command | Description |
|---------|-------------|
| `devpilot init` | Interactive project setup wizard |
| `devpilot login <service>` | Authenticate with a service (`trello`, `gmail`, `slack`) |
| `devpilot logout <service>` | Remove stored credentials |
| `devpilot status` | Show authentication status |
| `devpilot push <file>` | Create a Trello card from a plan markdown file |
| `devpilot run` | Autonomously process tasks from a Trello board |
| `devpilot sync` | Sync OpenSpec changes to Trello board or GitHub Issues |
| `devpilot gmail list` | List emails with search filters |
| `devpilot gmail read <id>` | Display full email content |
| `devpilot gmail mark-read <id>...` | Mark emails as read |
| `devpilot gmail bulk-mark-read` | Bulk mark emails as read by query |
| `devpilot gmail summary` | AI-powered email digest via Claude |
| `devpilot slack send` | Send message to a Slack channel or DM |
| `devpilot commit` | Generate a commit message from staged changes |
| `devpilot readme` | Generate or improve README.md |

### `devpilot push` Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--board` | *(required)* | Trello board name |
| `--list` | `Ready` | Target list name |

### `devpilot run` Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--board` | *(required)* | Trello board name |
| `--interval` | `300` | Poll interval in seconds |
| `--timeout` | `30` | Per-task timeout in minutes |
| `--review-timeout` | `10` | Code review timeout in minutes (0 to disable) |
| `--once` | `false` | Process one card and exit |
| `--dry-run` | `false` | Print actions without executing |
| `--no-tui` | `false` | Disable TUI dashboard |

**Multi-agent note:** Agents are configured in `.devpilot.yaml` under the `agents:` key (see `.devpilot.yaml` format below). The runner automatically uses `MultiRunner` when multiple agents are configured.

### `devpilot sync` Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--board` | *(from config)* | Override Trello board name |
| `--source` | `trello` | Task source (`trello` or `github`) |

### `devpilot gmail list` Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--unread` | `false` | Show only unread emails |
| `--after` | | Filter emails after date (YYYY-MM-DD) |
| `--limit` | `20` | Maximum number of emails to list |

### `devpilot gmail bulk-mark-read` Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--query` | *(required)* | Gmail search query (e.g. `category:promotions`) |

### `devpilot gmail summary` Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--channel` | | Send summary to a Slack channel |
| `--dm` | | Send summary as DM to a Slack user ID |
| `--no-mark-read` | `true` (without `--channel`/`--dm`) | Don't mark emails as read (default when no output target) |

### `devpilot slack send` Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--channel` | *(required)* | Channel name or user ID for DM |
| `--message` | | Message text (reads from stdin if omitted) |

## Configuration

### `.devpilot.yaml` Format

```yaml
# Project name (optional, auto-detected if omitted)
name: my-project

# Trello board (optional; can be overridden with --board flag)
board: "Sprint Board"

# Task source: trello or github (default: trello)
source: trello

# Multi-agent configuration (optional; defaults to ["claude"])
agents:
  - name: claude             # Claude Code agent
    model: claude-opus-4-6   # Optional: Claude model override
  - name: gemini             # Gemini CLI agent
  - name: opencode           # Opencode agent
  - name: cursor             # Cursor Agent CLI

# (Optional) Build and test commands auto-detected by devpilot init
build: go build -o bin/myapp ./cmd/myapp
test: go test ./...
```

**Example: Single agent (backward compatible)**

```yaml
board: "My Board"
```

**Example: Multi-agent (parallel execution)**

```yaml
board: "My Board"
agents:
  - name: claude
  - name: gemini
  - name: opencode
```

## Task Runner Workflow

The runner uses Trello lists as a state machine:

```
Ready → In Progress → Done
                    → Failed
```

For each card (per agent, in parallel if multiple agents configured):
1. Polls "Ready" list and sorts by priority (P0 > P1 > P2; default P2)
2. Claims card by moving to "In Progress" (distributed lock across all agents)
3. Validates the card has a description (the plan)
4. Creates branch `task/{cardID}-{slug}` from main
5. Runs the configured agent (`claude -p`, `gemini -p`, `opencode run`, etc.) with the plan
6. Pushes branch and creates a PR via `gh`
7. Optionally runs automated code review via a second agent invocation
8. Auto-merges PR (`gh pr merge --squash --auto`)
9. Moves card to "Done" (with PR link) or "Failed" (with error details)

Per-card logs: `~/.config/devpilot/logs/{card-id}.log`

### TUI Dashboard

In TTY mode, the runner displays a real-time terminal dashboard:

```
┌─────────────────────────────────────────────┐
│ Header: Board / Phase / Token Stats         │
├──────────────────────┬──────────────────────┤
│ Trello Lists Status  │ Active Card Info     │
├──────────────────────┼──────────────────────┤
│ Tool Call History     │ Files Read/Edited    │
├──────────────────────┴──────────────────────┤
│ Claude Text Output (scrollable)             │
├─────────────────────────────────────────────┤
│ Footer: Completed Tasks / Errors            │
└─────────────────────────────────────────────┘
```

Keys: `q`/`Ctrl-C` quit, `Tab` switch pane, `j/k/↑/↓` scroll, `g/G` top/bottom.

### Multi-Agent Execution

Run multiple AI agents in parallel to process tasks from the same Trello board. Each agent works independently — Trello's "move to In Progress" acts as a distributed lock, so agents claim different cards automatically.

**Configure agents in `.devpilot.yaml`:**

```yaml
agents:
  - name: claude           # Claude Code (default if omitted)
  - name: gemini           # Gemini CLI
  - name: opencode         # Opencode
  - name: cursor           # Cursor Agent CLI
```

**Example: Run with Claude and Gemini together**

```bash
# 1. Ensure both Claude Code and Gemini CLI are installed and authenticated
which claude gemini

# 2. Edit .devpilot.yaml to include both agents
cat >> .devpilot.yaml << 'EOF'
agents:
  - name: claude
  - name: gemini
EOF

# 3. Run the task runner — both agents poll in parallel
devpilot run --board "Sprint Board"
```

The TUI automatically adapts:
- **Single agent:** Original dashboard layout (status + tools + files + output)
- **Two agents:** Side-by-side columns with separate pane headers
- **Three+ agents:** Stacked vertical rows, one per agent

Each agent has its own:
- Tool call history and durations
- Output scrolling (`j/k` or `↑/↓`)
- Token usage stats
- File tracking (read/edited)

Keyboard shortcuts in multi-agent mode:
- `Tab` — cycle through agents
- `1`, `2`, `3`, `4` — jump directly to agent pane
- `[Tab]` within an agent — switch between tools and output (single agent only)

**Agent setup:**

| Agent | Command | Install | Notes |
|-------|---------|---------|-------|
| Claude | `claude -p --output-format stream-json --allowedTools=*` | [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) | Default; provides `/opsx:apply` skill for OpenSpec |
| Gemini | `gemini -p --output-format stream-json --yolo` | `curl https://files.genai.google.dev/gemini/install.sh \| bash` | Requires Gemini API key in env |
| Opencode | `opencode run --format json` | `npm install -g @google/opencode` | Text-only output (step-based JSON) |
| Cursor | `cursor-agent -p --output-format stream-json --force` | [Cursor Agent](https://www.cursor.com/) | Requires Cursor auth |

**OpenSpec with multiple agents:**

- **Claude:** Automatically uses `/opsx:apply <change-name>` skill (built-in)
- **Other agents:** Receive raw `proposal.md` + `tasks.md` content injected into the prompt (no skill system)

Both paths support resumability — interrupted tasks pick up from the last unchecked task.

## Architecture

DevPilot turns **markdown plans into shipped code** by orchestrating three systems: a task queue (Trello), an AI coding agent (`claude -p`), and standard Git/GitHub workflows.

### Event-Driven Pipeline

```
┌──────────────────────────────────────────────────────────┐
│  MultiRunner (N agents in parallel)                      │
│  Each agent polls Trello → manages card lifecycle        │
├──────────────────────────────────────────────────────────┤
│  Agent Adapters (pluggable per-agent logic)              │
│  • Claude: claude -p --output-format stream-json         │
│  • Gemini: gemini -p --output-format stream-json         │
│  • Opencode: opencode run --format json                  │
│  • Cursor: cursor-agent -p --output-format stream-json   │
├──────────────────────────────────────────────────────────┤
│  Output Bridges (translator per agent)                   │
│  Parses agent's JSON → translates to unified events      │
├──────────────────────────────────────────────────────────┤
│  TUI / Logger (consumers)                                │
│  Routes events by AgentName → renders multi-pane / logs  │
└──────────────────────────────────────────────────────────┘
```

- **MultiRunner** spawns N independent Runner goroutines; all poll the same Trello board
- **Trello as distributed lock:** "move to In Progress" is atomic — only one agent claims each card
- **Agent Adapters** translate agent-specific invocation logic and output format into a unified interface
- **Output Bridges** parse each agent's JSON format and emit typed runner events (`ToolStart`, `TextOutput`, etc.)
- **TUI** routes events by `AgentName` field to per-agent `agentPaneState`, rendering multi-agent dashboard
- **Logger** prefixes events with `[agentName]` in plain text mode

## Built-in Skills

DevPilot ships with Claude Code skills in `.claude/skills/`:

| Skill | Description |
|-------|-------------|
| `devpilot:pm` | Product manager — market research, competitor analysis, feature prioritization |
| `devpilot:trello` | Direct Trello board and card management from Claude Code |
| `devpilot:task-executor` | Autonomous plan execution (used internally by `devpilot run`) |
| `devpilot:task-refiner` | Improve and expand Trello card task plans |
| `devpilot:confluence-reviewer` | Review Atlassian Confluence pages and leave comments |

## Project Structure

```
devpilot/
├── cmd/devpilot/            CLI entry point
├── internal/
│   ├── auth/                Authentication & credential management (OAuth 2.0)
│   ├── generate/            AI-powered commit & readme generation
│   ├── gmail/               Gmail API client, email listing & AI summary
│   ├── initcmd/             Project initialization wizard
│   ├── openspec/            OpenSpec integration & sync command
│   ├── project/             Project config (.devpilot.yaml)
│   ├── slack/               Slack API client & message sending
│   ├── trello/              Trello API client & push command
│   └── taskrunner/          Runner, executor, TUI dashboard
├── .claude/skills/          Built-in Claude Code skills
├── .github/workflows/       CI/CD (test + release pipelines)
├── docs/plans/              Design & implementation plans
├── Makefile                 Build targets
└── CLAUDE.md                Project instructions for Claude Code
```

## Tech Stack

- **Language:** Go 1.25.6
- **CLI framework:** [Cobra](https://github.com/spf13/cobra)
- **TUI:** [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- **AI engine:** [Claude Code](https://claude.ai/code) (`claude -p` headless mode)
- **Task queue:** [Trello API](https://developer.atlassian.com/cloud/trello/)
- **Git/CI:** GitHub CLI (`gh`) for PRs and auto-merge

## Development

```bash
make build    # Build binary to bin/devpilot
make test     # Run all tests
make clean    # Remove build artifacts
```

## License

MIT
