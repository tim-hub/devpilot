## 1. Trello Custom Field Setup

- [x] 1.1 Add `ensureClaimFieldExists()` method to Trello client to auto-create "Claimed By" custom field if missing
- [x] 1.2 Update `runWithTUI()` and `runPlainText()` in commands.go to call `ensureClaimFieldExists()` during runner startup (after Trello source init)
- [x] 1.3 Handle Trello API errors gracefully: log warning if field creation fails and continue without blocking

## 2. Task Claiming Mechanism

- [x] 2.1 Add `claimCard()` method to Runner that sets "Claimed By" field to `{agent-name}:{timestamp-ms}` when moving card to "In Progress"
- [x] 2.2 Update Runner's `processCard()` to call `claimCard()` immediately after successfully moving to "In Progress" (before any other operations)
- [x] 2.3 Ensure claim timestamp is captured accurately (Unix milliseconds)

## 3. Ownership Verification

- [x] 3.1 Add `verifyCardOwnership()` method to Runner that re-fetches the card and checks "Claimed By" field matches our claim
- [x] 3.2 Add `ClaimCollisionEvent` event type to events.go with fields: CardID, CardName, OurAgentName, ActualAgentName, OurTimestamp, ActualTimestamp
- [x] 3.3 Update Runner's `processCard()` to call `verifyCardOwnership()` before `git.CreateBranch()` (right before the irreversible operation)
- [x] 3.4 If verification fails, emit `ClaimCollisionEvent`, log message, move back to "Ready" list, and return cleanly (skip to next card)

## 4. Execution Flow Updates

- [x] 4.1 Modify `processCard()` flow: claim → validate no empty description → verify ownership → branch creation → execution
- [x] 4.2 Ensure that if verification fails at step 3.4, the card is moved back to "Ready" (not left in "In Progress")
- [x] 4.3 Test that claim collision doesn't trigger `failCard()` (no error logged, just observability event)

## 5. Logging and Observability

- [x] 5.1 Update plain-text event handler in commands.go to log `ClaimCollisionEvent` with format `[agent] [claim-collision] Card 'name' already claimed by other-agent`
- [x] 5.2 Update TUI to route `ClaimCollisionEvent` to the appropriate pane and display as an info-level message
- [x] 5.3 Include claim information (timestamp, agent name) in execution logs (saveLog function)

## 6. Graceful Degradation

- [x] 6.1 Single-agent mode: skip field creation and verification if only one agent is configured; verify backward compatibility
- [x] 6.2 Field unavailable: if Trello API returns an error during claim or verification, log warning and fall back to current behavior (no collision detection)

## 7. Testing

- [ ] 7.1 Unit test: `claimCard()` correctly sets "Claimed By" field with proper format
- [ ] 7.2 Unit test: `verifyCardOwnership()` correctly detects ownership match and mismatch
- [ ] 7.3 Unit test: claim collision handling returns cleanly without error
- [ ] 7.4 Integration test: simulate two runners claiming same card; verify only one executes
- [ ] 7.5 Integration test: verify single-agent mode works without custom field (backward compatibility)
- [ ] 7.6 Integration test: custom field auto-creation on first multi-agent run

## 8. Documentation

- [ ] 8.1 Update CLAUDE.md to document the distributed locking mechanism and claim collision observable behavior
- [ ] 8.2 Add comments to `claimCard()` and `verifyCardOwnership()` explaining the race condition mitigation
- [ ] 8.3 Document that manual card manipulation during runs is unsupported and may cause collision issues
- [ ] 8.4 Update README if needed to mention that multi-agent mode uses Trello custom fields for coordination

## 9. Manual Testing

- [ ] 9.1 Manual test: run with two agents on same board; verify each agent gets different cards
- [ ] 9.2 Manual test: trigger a claim collision scenario (e.g., by manually moving card to Ready while two agents are starting); verify proper recovery
- [ ] 9.3 Manual test: agent crash during claimed task; verify card remains in "In Progress" (for cleanup documentation)
- [ ] 9.4 Manual test: single-agent mode with multi-agent config disabled; verify no "Claimed By" field is created

## 10. Finalization

- [ ] 10.1 Code review: verify all error paths are handled
- [ ] 10.2 Run full test suite to ensure no regressions
- [ ] 10.3 Build and verify binary compiles without errors
