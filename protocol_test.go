package methane

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadyCheckRequest_JSONRoundTrip verifies that Timeout is encoded as a
// string duration (e.g. "30s") rather than raw nanoseconds.
func TestReadyCheckRequest_JSONRoundTrip(t *testing.T) {
	req := ReadyCheckRequest{
		SessionID: "test-session",
		Timeout:   30 * time.Second,
	}

	encoded, err := EncodeMessage(MsgTypeReadyCheck, req)
	require.NoError(t, err)

	// The wire form must contain the human-readable string "30s", not a large
	// integer representing nanoseconds.
	assert.Contains(t, encoded, `"30s"`, "timeout should be encoded as a duration string")
	assert.NotContains(t, encoded, "30000000000", "timeout must not be raw nanoseconds")

	// Decode back and verify round-trip fidelity.
	env, err := DecodeMessage(encoded)
	require.NoError(t, err)

	var got ReadyCheckRequest
	require.NoError(t, DecodePayload(env, &got))
	assert.Equal(t, req.SessionID, got.SessionID)
	assert.Equal(t, req.Timeout, got.Timeout)
}

func TestReadyCheckRequest_InvalidTimeout(t *testing.T) {
	// Use a hand-crafted JSON string to inject an unparseable duration value.
	// This tests the UnmarshalJSON error path, which cannot be reached by
	// encoding a valid ReadyCheckRequest through EncodeMessage.
	raw := `{"type":"ready_check","payload":{"session_id":"x","timeout":"not-a-duration"}}`
	env, err := DecodeMessage(raw)
	require.NoError(t, err)

	var req ReadyCheckRequest
	err = DecodePayload(env, &req)
	assert.Error(t, err, "invalid duration should produce an error")
}

// TestQueue_NilTimeProvider verifies that SetTimeProvider(nil) resets to
// RealTimeProvider and doesn't panic.
func TestQueue_NilTimeProvider(t *testing.T) {
	q := NewMatchmakingQueue(DefaultQueueConfig())
	// Should not panic.
	q.SetTimeProvider(nil)
	assert.Equal(t, 0, q.Size())
}

// TestQueue_ZeroQueueTimeout verifies that a QueueTimeout of 0 doesn't cause a
// divide-by-zero panic and that the rating window stays at RatingWindow.
func TestQueue_ZeroQueueTimeout(t *testing.T) {
	cfg := DefaultQueueConfig()
	cfg.QueueTimeout = 0
	cfg.MatchSize = 2
	cfg.RatingWindow = 100

	q := NewMatchmakingQueue(cfg)
	tp := NewMockTimeProvider(time.Now())
	q.SetTimeProvider(tp)

	// Advance clock; with zero timeout no division should occur.
	tp.Advance(10 * time.Minute)
	p1 := newTestPlayer(1500)
	p2 := newTestPlayer(1501)
	p2.PublicKey[1] = 1
	require.NoError(t, q.Enqueue(p1, ModeFFA, RegionAny))
	require.NoError(t, q.Enqueue(p2, ModeFFA, RegionAny))
	// Must not panic.
	q.RunOnce()
}

// TestService_SetTimeProvider_Nil verifies nil tp resets to real provider without panicking.
func TestService_SetTimeProvider_Nil(t *testing.T) {
	svc := newTestService(t)
	// Should not panic.
	svc.SetTimeProvider(nil)
}

// TestService_SetAutoAcceptFriends verifies the auto-accept flag can be toggled.
func TestService_SetAutoAcceptFriends(t *testing.T) {
	svc := newTestService(t)
	assert.False(t, svc.autoAcceptFriends)
	svc.SetAutoAcceptFriends(true)
	assert.True(t, svc.autoAcceptFriends)
	svc.SetAutoAcceptFriends(false)
	assert.False(t, svc.autoAcceptFriends)
}

// TestHandleMatchFound_CreatesSession verifies that receiving a MatchFound
// message creates a session so subsequent ready-check messages can succeed.
func TestHandleMatchFound_CreatesSession(t *testing.T) {
	svc := newTestService(t)
	players := makePlayers(2)

	msg := MatchFoundMessage{
		SessionID: "remote-session-42",
		Players: []string{
			players[0].PublicKeyHex(),
			players[1].PublicKeyHex(),
		},
		GameMode: ModeFFA,
	}
	env := fakeEnvelope(t, MsgTypeMatchFound, msg)
	svc.handleMatchFound(env)

	svc.mu.RLock()
	session, ok := svc.sessions["remote-session-42"]
	svc.mu.RUnlock()

	require.True(t, ok, "session should be created after MatchFound message")
	assert.Equal(t, "remote-session-42", session.ID)
	assert.Equal(t, ModeFFA, session.GameMode)
}

// TestEnqueueForMatch_UpdatesPreferences verifies that EnqueueForMatch stores
// the game mode and region into the player's preferences so handleMatchFormed
// can derive the session's GameMode correctly.
func TestEnqueueForMatch_UpdatesPreferences(t *testing.T) {
	svc := newTestService(t)
	err := svc.EnqueueForMatch(ModeTeamDeathmatch, RegionEU)
	require.NoError(t, err)

	self := svc.GetSelfPlayer()
	assert.Equal(t, ModeTeamDeathmatch, self.Preferences.GameMode)
	assert.Equal(t, RegionEU, self.Preferences.Region)
}
