## Why

With multiple agents polling the same Trello board concurrently, there is a race condition where two agents can both claim the same card and begin execution simultaneously. While Trello's "move to In Progress" API call is atomic on the server, the distributed nature of agent goroutines polling in parallel creates a small window where both agents can fetch the Ready list, see the same card, and both attempt to move it to In Progress. Only one succeeds, but both may have already initiated processing. This can lead to:

- Duplicate branch creation and code execution
- Conflicting PRs from different agents on the same task
- Wasted compute and confusing task history

We need an explicit locking mechanism to prevent multiple agents from claiming the same card.

## What Changes

- Add a "claimed by" Trello custom field to track which agent owns a card
- Implement a claim-and-verify pattern: check the field before starting execution to ensure we own the card
- Add retry logic: if another agent beat us to the card, skip it and move to the next one
- Track attempted claims and provide observability into claim collisions
- Update the runner to validate card ownership before branch creation

## Capabilities

### New Capabilities

- `distributed-task-locking`: Mechanism to ensure exactly one agent claims each task, with claim verification before execution starts

### Modified Capabilities

- `multi-agent-execution`: Update to use distributed locking to prevent duplicate task processing

## Impact

- **Files modified**: `internal/taskrunner/runner.go`, `internal/taskrunner/commands.go`, `internal/trello/client.go`
- **Trello API**: Requires setting/reading custom fields on cards (supported by Trello API)
- **Agent goroutines**: No change to structure; locking is transparent
- **Observable**: Claim collisions logged; agents skip cards already claimed by others
