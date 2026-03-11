## Context

The multi-agent runner spawns N independent goroutines, each polling the same Trello "Ready" list concurrently. Each agent fetches the list, iterates through cards, and attempts to move them to "In Progress". While Trello's move operation is atomic on the server, the inherent delay between polling and claim attempts creates a window where two agents can both see the same card and both call the move operation. In distributed systems, this is a classic race condition: the server may process the first move successfully while rejecting the second, but both agents may have already begun execution based on their local view of the card.

The current implementation relies solely on Trello's move operation succeeding for one agent and failing for the other, with no explicit verification or lock mechanism. This leaves a gap.

## Goals / Non-Goals

**Goals:**
- Ensure exactly one agent claims and executes each task, even under concurrent polling
- Detect and handle claim collisions gracefully (skip and move to next card)
- Provide visibility into which agent claimed each task
- Implement using Trello's built-in capabilities (no external database or service)
- Maintain backward compatibility with single-agent mode

**Non-Goals:**
- Distributed consensus or consensus protocols
- Complex transaction semantics
- Support for agents outside the devpilot runner (e.g., manual Trello operations during runs)
- Pre-reservation or bidding mechanisms
- Implement on GitHub Issues or other task sources (focus on Trello first)

## Decisions

### 1. Use Trello Custom Fields for Ownership Tracking

**Decision:** Store the claiming agent's name and timestamp in a Trello custom field named "Claimed By" on each card.

**Rationale:**
- Trello's custom fields are part of the card data model and persist reliably
- Setting a field is an atomic operation on Trello's server
- No external infrastructure required
- Integrates naturally with existing Trello-based workflow
- Easy to inspect manually for debugging

**Alternatives considered:**
- Local file-based locks: Unreliable across distributed processes; susceptible to filesystem issues
- External Redis/database: Adds infrastructure complexity; overkill for this use case
- Message queue (FIFO task claiming): Introduces new external dependency

### 2. Claim-and-Verify Pattern with Optimistic Locking

**Decision:** Agents use a two-step process:
1. **Claim phase** (polling loop): Set "Claimed By" field to `{agent-name}:{timestamp}` when moving to "In Progress"
2. **Verify phase** (before execution): Before branch creation, re-fetch the card and confirm "Claimed By" field still matches our agent; if not, abort and move to next card

**Rationale:**
- Optimistic locking minimizes waiting and blocking
- Verification happens right before the irreversible operation (branch creation), catching claim conflicts early
- Low overhead: one extra API call per claimed card
- Agents don't block each other; they compete naturally via Trello

**Alternatives considered:**
- Pessimistic locking: Hold the lock until task completes (heavyweight, prone to deadlock if agent crashes)
- No verification: Rely solely on move-to-In-Progress succeeding (current approach; insufficient)

### 3. Skip-on-Collision Recovery Strategy

**Decision:** If verification fails (another agent beat us to the card), log an event and skip the card. The agent moves to the next card in the next poll cycle.

**Rationale:**
- Collisions are rare; skipping is lightweight
- No need for retry backoff or queuing logic
- Other cards are always available; throughput is maintained
- Simpler than implementing retry/backoff; avoids starvation risk

**Alternatives considered:**
- Retry with exponential backoff: Adds complexity; marginal benefit given rareness
- Queue-based claiming: Centralized bottleneck; defeats parallelism advantage

### 4. Auto-create Custom Field on First Run

**Decision:** When runner starts with multiple agents, check if "Claimed By" field exists on the board. If not, create it automatically.

**Rationale:**
- Graceful setup; no manual configuration required
- Backward compatible; single-agent mode doesn't require the field

**Alternatives considered:**
- Require manual field creation: Adds user friction
- Fail if field missing: Poor UX for existing boards

### 5. Field Format and Content

**Decision:** Store `"{agent-name}:{unix-timestamp-ms}"` in the "Claimed By" field (e.g., `"claude:1710250800000"`).

**Rationale:**
- Agent name is human-readable and useful for debugging
- Timestamp allows observability (when was this card claimed?)
- Unix milliseconds avoids timezone issues and is standard in Go

## Risks / Trade-offs

- **Risk: Trello API rate limits** → Claim-and-verify adds one extra read per claimed card; under normal load (few agents, few cards) this is negligible. In extreme cases (100+ agents), may hit Trello rate limits; mitigation: implement exponential backoff on 429 responses.

- **Risk: Stale field values if Trello API fails** → If setting "Claimed By" succeeds but the response is lost, we may not know we own the card. Mitigation: follow up with a fetch to verify.

- **Risk: Manual card manipulation** → If a user manually moves a card or edits the "Claimed By" field during a run, agents may become confused. Mitigation: documented as unsupported; users should not manually edit during active runs.

- **Risk: Agent crashes without cleanup** → If an agent crashes after claiming but before completion, the "Claimed By" field remains set; other agents see the card as "taken". Mitigation: either accept the task is lost (low impact; card can be manually reset), or implement a timeout (claim expires after N minutes if task not marked Done).

- **Trade-off: Single claim point of truth vs. distributed trust** → By using Trello's field, we depend on Trello as the source of truth. This is acceptable for a Trello-centric tool but limits portability to other task sources without reimplementation.

## Migration Plan

1. Add Trello custom field creation logic to `devpilot run` startup
2. Implement `claimCard()` method in Runner that sets "Claimed By" field
3. Implement `verifyOwnership()` method that re-fetches and checks field
4. Update `processCard()` to call `verifyOwnership()` before `git.CreateBranch()`
5. Log `ClaimCollisionEvent` when verification fails
6. Test with two agents on the same board to verify only one executes per card

**Rollback:** Remove the verification call; system falls back to current behavior (Trello move-to-In Progress as sole lock).

## Open Questions

- Should we implement a timeout for "Claimed By" values (e.g., consider a claim stale after 2x task timeout)? Or accept that stuck claims must be manually cleared?
- Should claim collisions be retried with backoff, or just skip and move on?
- How should we handle claims set by agents that are no longer running (e.g., old timestamps)?
