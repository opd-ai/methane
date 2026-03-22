package methane

import (
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// NewGameSession creates a GameSession for the given players and game mode.
//
//export NewGameSession
func NewGameSession(players []*Player, mode GameMode) *GameSession {
	readyAcks := make(map[string]bool, len(players))
	for _, p := range players {
		readyAcks[hex.EncodeToString(p.PublicKey[:])] = false
	}
	return &GameSession{
		ID:        generateSessionID(),
		Players:   players,
		State:     MatchFormed,
		GameMode:  mode,
		CreatedAt: time.Now(),
		ReadyAcks: readyAcks,
		Results:   make(map[string]Outcome),
		tp:        RealTimeProvider{},
	}
}

// generateSessionID produces a unique session identifier.
func generateSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}

// GameSession tracks the lifecycle of a single matched game.
type GameSession struct {
	// ID is the unique session identifier.
	ID string
	// Players is the list of matched players.
	Players []*Player
	// State is the current lifecycle state.
	State SessionState
	// GameMode is the game mode being played.
	GameMode GameMode
	// CreatedAt is the time the session was created.
	CreatedAt time.Time
	// ReadyAcks maps hex public keys to their ready status.
	ReadyAcks map[string]bool
	// Results maps hex public keys to their game outcome.
	Results map[string]Outcome
	mu      sync.RWMutex
	tp      TimeProvider
}

// SetTimeProvider injects a TimeProvider for testing.
func (s *GameSession) SetTimeProvider(tp TimeProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tp = tp
}

// StartReadyCheck transitions the session to the ReadyCheck state.
// timeout specifies how long players have to confirm readiness.
func (s *GameSession) StartReadyCheck(_ time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = ReadyCheck
}

// RecordReadyAck records a player's ready (or not-ready) response.
func (s *GameSession) RecordReadyAck(playerPK [32]byte, ready bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ReadyAcks[hex.EncodeToString(playerPK[:])] = ready
}

// IsReadyCheckComplete returns true if all players have responded to the ready check.
func (s *GameSession) IsReadyCheckComplete() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.ReadyAcks {
		if !r {
			return false
		}
	}
	return len(s.ReadyAcks) > 0
}

// AllPlayersReady returns true if all players have confirmed readiness.
func (s *GameSession) AllPlayersReady() bool {
	return s.IsReadyCheckComplete()
}

// StartGame transitions the session to the Launching state.
func (s *GameSession) StartGame(_ *GameLaunchInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = Launching
}

// RecordResult records the final game results and transitions to InProgress.
func (s *GameSession) RecordResult(result *GameResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result.SessionID != s.ID {
		return fmt.Errorf("session ID mismatch: got %s want %s", result.SessionID, s.ID)
	}
	for pk, outcome := range result.Results {
		s.Results[pk] = outcome
	}
	s.State = InProgress
	return nil
}

// Complete marks the session as completed.
func (s *GameSession) Complete() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = Completed
}

// Cancel marks the session as cancelled.
func (s *GameSession) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = Cancelled
}
