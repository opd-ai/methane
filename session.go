package methane

import (
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

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
	// ReadyAcks maps hex public keys to their last reported ready status.
	// To query ready-check completion, use IsReadyCheckComplete/AllPlayersReady
	// instead of reading this map directly, since a false entry may mean either
	// "not ready" or "not yet responded".
	ReadyAcks map[string]bool
	// ReadyDeadline is when the ready-check expires; zero means no deadline set.
	ReadyDeadline time.Time
	// Results maps hex public keys to their game outcome.
	Results   map[string]Outcome
	responded map[string]struct{} // tracks which players have sent any response
	mu        sync.RWMutex
	tp        TimeProvider
}

// newGameSessionWithID creates a GameSession using an explicit session ID and
// TimeProvider. It is used both by NewGameSession and when reconstructing a
// remote session received via a MatchFoundMessage.
func newGameSessionWithID(id string, players []*Player, mode GameMode, tp TimeProvider) *GameSession {
	readyAcks := make(map[string]bool, len(players))
	for _, p := range players {
		readyAcks[hex.EncodeToString(p.PublicKey[:])] = false
	}
	sess := &GameSession{
		ID:        id,
		Players:   players,
		State:     MatchFormed,
		GameMode:  mode,
		ReadyAcks: readyAcks,
		responded: make(map[string]struct{}),
		Results:   make(map[string]Outcome),
		tp:        tp,
	}
	sess.CreatedAt = tp.Now()
	return sess
}

// NewGameSession creates a GameSession for the given players and game mode.
//
//export NewGameSession
func NewGameSession(players []*Player, mode GameMode) *GameSession {
	tp := TimeProvider(RealTimeProvider{})
	id := fmt.Sprintf("session-%d", tp.Now().UnixNano())
	return newGameSessionWithID(id, players, mode, tp)
}

// SetTimeProvider injects a TimeProvider for deterministic testing.
// Passing nil resets to the real wall-clock provider.
func (s *GameSession) SetTimeProvider(tp TimeProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tp == nil {
		s.tp = RealTimeProvider{}
		return
	}
	s.tp = tp
}

// StartReadyCheck transitions the session to the ReadyCheck state and records
// the deadline by which all players must respond.
func (s *GameSession) StartReadyCheck(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = ReadyCheck
	s.ReadyDeadline = s.tp.Now().Add(timeout)
}

// RecordReadyAck records a player's ready (or not-ready) response.
// It marks the player as having responded regardless of their answer so that
// IsReadyCheckComplete can distinguish "no response" from "declined".
func (s *GameSession) RecordReadyAck(playerPK [32]byte, ready bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pkHex := hex.EncodeToString(playerPK[:])
	s.ReadyAcks[pkHex] = ready
	s.responded[pkHex] = struct{}{}
}

// allRespondedLocked returns true when every player in the session has sent a
// ready-check response. Must be called with s.mu held (read or write).
func (s *GameSession) allRespondedLocked() bool {
	return len(s.Players) > 0 && len(s.responded) == len(s.Players)
}

// IsReadyCheckComplete returns true if every player has sent a response
// (ready or not-ready). Returns false when no players are present.
func (s *GameSession) IsReadyCheckComplete() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.allRespondedLocked()
}

// AllPlayersReady returns true if every player has responded and all responses
// were positive. It is a stricter test than IsReadyCheckComplete.
func (s *GameSession) AllPlayersReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.allRespondedLocked() {
		return false
	}
	for _, r := range s.ReadyAcks {
		if !r {
			return false
		}
	}
	return true
}

// StartGame transitions the session to the Launching state.
func (s *GameSession) StartGame(_ *GameLaunchInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = Launching
}

// RecordResult records the final game results and transitions the session to
// Completed. Calling Complete() afterwards is harmless.
func (s *GameSession) RecordResult(result *GameResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result.SessionID != s.ID {
		return fmt.Errorf("session ID mismatch: got %s want %s", result.SessionID, s.ID)
	}
	for pk, outcome := range result.Results {
		s.Results[pk] = outcome
	}
	s.State = Completed
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
