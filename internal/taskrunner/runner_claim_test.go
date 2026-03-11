package taskrunner

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/siyuqian/devpilot/internal/trello"
)

// TestMultiAgentClaimCollisionDetection verifies that when two agents attempt to claim
// the same card, one succeeds and the other detects the collision and skips.
func TestMultiAgentClaimCollisionDetection(t *testing.T) {
	// Mock Trello API server
	var mu sync.Mutex
	cardClaims := make(map[string]string) // cardID -> claim value (agent:timestamp)
	moveHistory := []string{}              // track card movements

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/1/members/me/boards":
			// Return a single board
			board := trello.Board{ID: "board1", Name: "Test Board"}
			json.NewEncoder(w).Encode([]trello.Board{board})

		case "/1/boards/board1/lists":
			// Return lists
			lists := []trello.List{
				{ID: "ready-list", Name: "Ready"},
				{ID: "inprog-list", Name: "In Progress"},
				{ID: "done-list", Name: "Done"},
				{ID: "failed-list", Name: "Failed"},
			}
			json.NewEncoder(w).Encode(lists)

		case "/1/lists/ready-list/cards":
			// Return a single card in Ready
			card := trello.Card{
				ID:       "card1",
				Name:     "Test Task",
				Desc:     "Do something",
				ShortURL: "https://trello.com/c/card1",
			}
			json.NewEncoder(w).Encode([]trello.Card{card})

		case "/1/cards/card1":
			// GET card details (for claim verification)
			if r.Method == "GET" {
				mu.Lock()
				claimValue := cardClaims["card1"]
				mu.Unlock()

				card := map[string]interface{}{
					"id":   "card1",
					"name": "Test Task",
				}
				if claimValue != "" {
					card["customFieldData"] = []map[string]interface{}{
						{
							"idCustomField": "claim-field-id",
							"value": map[string]interface{}{
								"text": claimValue,
							},
						},
					}
				}
				json.NewEncoder(w).Encode(card)
			}
			// PUT to move card (handled below)

		case "/1/boards/board1/customFields":
			// Return existing or create new custom field
			if r.Method == "GET" {
				fields := []map[string]interface{}{
					{
						"id":   "claim-field-id",
						"name": "Claimed By",
						"type": "text",
					},
				}
				json.NewEncoder(w).Encode(fields)
			} else if r.Method == "POST" {
				field := map[string]interface{}{
					"id":   "claim-field-id",
					"name": "Claimed By",
				}
				json.NewEncoder(w).Encode(field)
			}

		default:
			// Handle card moves and claim updates
			if r.Method == "PUT" {
				r.ParseForm()
				if listID := r.FormValue("idList"); listID != "" {
					mu.Lock()
					if listID == "inprog-list" {
						moveHistory = append(moveHistory, "moved to In Progress")
					} else if listID == "ready-list" {
						moveHistory = append(moveHistory, "moved back to Ready")
					}
					mu.Unlock()
				}
				if claimValue := r.FormValue("value"); claimValue != "" {
					mu.Lock()
					cardClaims["card1"] = claimValue
					mu.Unlock()
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{}"))
				return
			}

			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// Create mock Trello source with the test server
	client := trello.NewClient("test-key", "test-token", trello.WithBaseURL(ts.URL))
	source := NewTrelloSource(client, "Test Board")

	// Initialize source (this populates list IDs)
	_, err := source.Init()
	if err != nil {
		t.Fatalf("failed to init source: %v", err)
	}

	// Simulate two agents claiming the same card
	var wg sync.WaitGroup
	var agent1Claim, agent2Claim string
	var agent1OwnsCard, agent2OwnsCard bool
	var collisionDetected bool

	// Agent 1
	wg.Add(1)
	go func() {
		defer wg.Done()
		cfg := Config{
			ClaimFieldID: "claim-field-id",
			Agents: []AgentConfig{
				{Name: "agent1"},
			},
		}
		runner := &Runner{
			config: cfg,
			source: source,
			logger: log.New(io.Discard, "", 0), // Discard logs
		}

		task := Task{ID: "card1", Name: "Test Task"}

		// Agent 1 claims the card
		agent1Claim = runner.claimCard(task)
		time.Sleep(50 * time.Millisecond) // Small delay to let agent2 see the claim

		// Agent 1 verifies ownership
		agent1OwnsCard = runner.verifyCardOwnership(task, agent1Claim)
	}()

	// Agent 2 (starts slightly after agent 1)
	time.Sleep(10 * time.Millisecond)
	wg.Add(1)
	go func() {
		defer wg.Done()
		cfg := Config{
			ClaimFieldID: "claim-field-id",
			Agents: []AgentConfig{
				{Name: "agent2"},
			},
		}
		runner := &Runner{
			config: cfg,
			source: source,
			logger: log.New(io.Discard, "", 0),
			eventHandler: func(e Event) {
				if _, ok := e.(ClaimCollisionEvent); ok {
					collisionDetected = true
				}
			},
		}

		task := Task{ID: "card1", Name: "Test Task"}

		// Agent 2 claims the card
		agent2Claim = runner.claimCard(task)
		time.Sleep(50 * time.Millisecond) // Small delay after claiming

		// Agent 2 verifies ownership (will detect collision)
		agent2OwnsCard = runner.verifyCardOwnership(task, agent2Claim)
	}()

	wg.Wait()

	// Verify results
	if agent1Claim == "" {
		t.Error("agent1 claim should not be empty")
	}
	if agent2Claim == "" {
		t.Error("agent2 claim should not be empty")
	}

	// Exactly one agent should own the card
	if agent1OwnsCard && agent2OwnsCard {
		t.Error("both agents claim ownership - race condition not prevented!")
	}
	if !agent1OwnsCard && !agent2OwnsCard {
		t.Error("neither agent owns the card - unexpected state")
	}

	// The agent that claims first should own it
	if agent1Claim == agent2Claim {
		t.Errorf("both agents have same claim (timestamps should differ): %s", agent1Claim)
	}

	// One of them should detect collision (the one that claimed second)
	if !collisionDetected {
		t.Error("expected collision detection event but none was emitted")
	}

	// Card should have been moved back to Ready after collision
	if len(moveHistory) == 0 {
		t.Error("expected card movement history but got none")
	}
}

// TestClaimCardAndVerifyWithoutCollision verifies the normal case where a single
// agent claims and verifies a card without collision.
func TestClaimCardAndVerifyWithoutCollision(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
	}))
	defer ts.Close()

	client := trello.NewClient("key", "token", trello.WithBaseURL(ts.URL))
	source := &TrelloSource{client: client}

	cfg := Config{
		ClaimFieldID: "field-id",
		Agents:       []AgentConfig{{Name: "agent1"}},
	}
	runner := &Runner{config: cfg, source: source, logger: log.New(io.Discard, "", 0)}

	task := Task{ID: "card1", Name: "Test Task"}

	// Claim the card
	claim := runner.claimCard(task)
	if claim == "" {
		t.Error("expected non-empty claim value")
	}

	// Verify ownership (with the same claim, should succeed)
	owns := runner.verifyCardOwnership(task, claim)
	if !owns {
		t.Error("expected to own the card with correct claim value")
	}
}

// TestSplitClaim verifies the claim parsing helper.
func TestSplitClaim(t *testing.T) {
	tests := []struct {
		claim           string
		expectedAgent   string
		expectedHasTime bool
	}{
		{"agent1:1234567890", "agent1", true},
		{"multi-agent:999999999999", "multi-agent", true},
		{"", "unknown", false},
		{"invalid-format", "unknown", false},
		{"no-timestamp:", "unknown", false},
	}

	for _, tc := range tests {
		parts := splitClaim(tc.claim)
		agent := parts[0].(string)
		if agent != tc.expectedAgent {
			t.Errorf("claim=%q: expected agent %q, got %q", tc.claim, tc.expectedAgent, agent)
		}
	}
}
