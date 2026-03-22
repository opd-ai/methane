package methane

import "time"

// SessionState represents the lifecycle state of a game session.
type SessionState uint8

const (
	// MatchFormed is the initial state when players have been matched.
	MatchFormed SessionState = iota
	// ReadyCheck is the state during the ready-check phase.
	ReadyCheck
	// Launching is the state when the game is being launched.
	Launching
	// InProgress is the state when the game is actively running.
	InProgress
	// Completed is the state when the session ended normally.
	Completed
	// Cancelled is the state when the session was cancelled.
	Cancelled
)

// GameMode represents the type of game being played.
type GameMode uint8

const (
	// ModeFFA is a free-for-all game mode.
	ModeFFA GameMode = iota
	// ModeTeamDeathmatch is a team deathmatch game mode.
	ModeTeamDeathmatch
	// ModeCaptureFlag is a capture-the-flag game mode.
	ModeCaptureFlag
	// ModeCooperative is a co-operative game mode.
	ModeCooperative
)

// Region constants for matchmaking geographic preferences.
const (
	// RegionAny accepts players from any region.
	RegionAny = "any"
	// RegionNA is the North America region.
	RegionNA = "na"
	// RegionEU is the Europe region.
	RegionEU = "eu"
	// RegionASIA is the Asia-Pacific region.
	RegionASIA = "asia"
)

// MatchFoundEvent is fired when enough players have been matched together.
type MatchFoundEvent struct {
	// SessionID is the unique identifier for the new game session.
	SessionID string
	// Players is the list of players included in the match.
	Players []*Player
}

// TimeProvider is an interface for obtaining the current time.
// It allows tests to inject a deterministic clock.
type TimeProvider interface {
	// Now returns the current time.
	Now() time.Time
}

// RealTimeProvider is a TimeProvider that returns the actual wall-clock time.
type RealTimeProvider struct{}

// Now returns the current wall-clock time.
func (RealTimeProvider) Now() time.Time {
	return time.Now()
}

// MockTimeProvider is a TimeProvider for use in tests with a fixed time.
type MockTimeProvider struct {
	current time.Time
}

// NewMockTimeProvider creates a MockTimeProvider starting at t.
func NewMockTimeProvider(t time.Time) *MockTimeProvider {
	return &MockTimeProvider{current: t}
}

// Now returns the mock's current time.
func (m *MockTimeProvider) Now() time.Time {
	return m.current
}

// Advance moves the mock clock forward by d.
func (m *MockTimeProvider) Advance(d time.Duration) {
	m.current = m.current.Add(d)
}

// Set sets the mock clock to t.
func (m *MockTimeProvider) Set(t time.Time) {
	m.current = t
}
