package methane

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMatchmakingService_GetOrCreatePlayer verifies player creation/lookup.
func TestMatchmakingService_GetOrCreatePlayer(t *testing.T) {
	svc := newTestService(t)
	var pk [32]byte
	pk[0] = 42
	p1 := svc.GetOrCreatePlayer(pk)
	require.NotNil(t, p1)
	p2 := svc.GetOrCreatePlayer(pk)
	assert.Same(t, p1, p2, "should return the same player instance")
}

// TestMatchmakingService_GetSelfPlayer verifies self-player creation.
func TestMatchmakingService_GetSelfPlayer(t *testing.T) {
	svc := newTestService(t)
	self := svc.GetSelfPlayer()
	require.NotNil(t, self)
	assert.Equal(t, svc.selfPK, self.PublicKey)
}

// TestMatchmakingService_SetTimeProvider verifies the time provider can be set.
func TestMatchmakingService_SetTimeProvider(t *testing.T) {
	svc := newTestService(t)
	tp := NewMockTimeProvider(time.Now())
	// Should not panic.
	svc.SetTimeProvider(tp)
}

// TestMatchmakingService_OnMatchFound registers and fires callback.
func TestMatchmakingService_OnMatchFound(t *testing.T) {
	svc := newTestService(t)
	fired := make(chan *MatchFoundEvent, 1)
	svc.OnMatchFound(func(ev *MatchFoundEvent) {
		fired <- ev
	})

	// Directly trigger the internal handleMatchFormed path via the queue callback.
	players := makePlayers(2)
	svc.handleMatchFormed(players)

	select {
	case ev := <-fired:
		assert.NotEmpty(t, ev.SessionID)
		assert.Len(t, ev.Players, 2)
	case <-time.After(time.Second):
		t.Fatal("match found callback not fired")
	}
}

// TestMatchmakingService_OnLobbyFound registers and fires callback.
func TestMatchmakingService_OnLobbyFound(t *testing.T) {
	svc := newTestService(t)
	found := make(chan *LobbyAdvertisement, 1)
	svc.OnLobbyFound(func(ad *LobbyAdvertisement) {
		found <- ad
	})

	ad := &LobbyAdvertisement{
		LobbyID:  1,
		GameName: "FooGame",
	}
	env := fakeEnvelope(t, MsgTypeLobbyAd, ad)
	svc.handleLobbyAd(env)

	select {
	case got := <-found:
		assert.Equal(t, "FooGame", got.GameName)
	case <-time.After(time.Second):
		t.Fatal("lobby found callback not fired")
	}
}

// TestMatchmakingService_EnqueueAndLeaveQueue tests queue integration.
func TestMatchmakingService_EnqueueAndLeaveQueue(t *testing.T) {
	svc := newTestService(t)
	err := svc.EnqueueForMatch(ModeFFA, RegionAny)
	require.NoError(t, err)
	assert.Equal(t, 1, svc.queue.Size())

	ok := svc.LeaveQueue()
	assert.True(t, ok)
	assert.Equal(t, 0, svc.queue.Size())
}

// TestMatchmakingService_CreateLobby_NoTox tests lobby creation state.
func TestMatchmakingService_CreateLobby_NoTox(t *testing.T) {
	// We can't call ConferenceNew without a live Tox; test ListLobbies with
	// a pre-populated lobby instead.
	svc := newTestService(t)
	// Inject a lobby directly.
	var pk [32]byte
	cfg := LobbyConfig{GameName: "Test", MaxPlayers: 4, Region: RegionAny}
	lobby := NewLobby(1, pk, cfg, RealTimeProvider{})
	svc.mu.Lock()
	svc.lobbies[1] = lobby
	svc.mu.Unlock()

	lobbies := svc.ListLobbies()
	assert.Len(t, lobbies, 1)
	assert.Equal(t, "Test", lobbies[0].Config.GameName)
}

// TestMatchmakingService_LeaveLobby_NotFound tests error on missing lobby.
func TestMatchmakingService_LeaveLobby_NotFound(t *testing.T) {
	svc := newTestService(t)
	err := svc.LeaveLobby(999)
	assert.Error(t, err)
}

// TestMatchmakingService_HandleFriendMessage_Unknown verifies unknown msgs are silently dropped.
func TestMatchmakingService_HandleFriendMessage_Unknown(t *testing.T) {
	svc := newTestService(t)
	// Should not panic on a plain string that's not a valid envelope.
	svc.handleFriendMessage(0, "not a json envelope")
}

// TestMatchmakingService_HandleReadyResponse tests ready response handling.
func TestMatchmakingService_HandleReadyResponse(t *testing.T) {
	svc := newTestService(t)
	players := makePlayers(2)
	session := NewGameSession(players, ModeFFA)
	svc.mu.Lock()
	svc.sessions[session.ID] = session
	svc.mu.Unlock()

	resp := ReadyCheckResponse{
		SessionID: session.ID,
		PlayerPK:  hex.EncodeToString(players[0].PublicKey[:]),
		Ready:     true,
	}
	env := fakeEnvelope(t, MsgTypeReadyResponse, resp)
	svc.handleReadyResponse(env)
	// Check that the ready ack was recorded.
	assert.True(t, session.ReadyAcks[players[0].PublicKeyHex()])
}

// TestMatchmakingService_HandleGameResult tests result recording.
func TestMatchmakingService_HandleGameResult(t *testing.T) {
	svc := newTestService(t)
	players := makePlayers(2)
	session := NewGameSession(players, ModeFFA)
	svc.mu.Lock()
	svc.sessions[session.ID] = session
	svc.mu.Unlock()

	result := GameResult{
		SessionID: session.ID,
		Results: map[string]Outcome{
			players[0].PublicKeyHex(): OutcomeWin,
			players[1].PublicKeyHex(): OutcomeLoss,
		},
	}
	env := fakeEnvelope(t, MsgTypeGameResult, result)
	svc.handleGameResult(env)
	assert.Equal(t, Completed, session.State)
}

// newTestService creates a MatchmakingService with just enough state for unit tests.
func newTestService(_ *testing.T) *MatchmakingService {
	var selfPK [32]byte
	selfPK[0] = 0xFF
	logger := logrus.New()
	svc := &MatchmakingService{
		lobbies:  make(map[uint32]*Lobby),
		sessions: make(map[string]*GameSession),
		queue:    NewMatchmakingQueue(DefaultQueueConfig()),
		players:  make(map[[32]byte]*Player),
		selfPK:   selfPK,
		logger:   logger,
		tp:       RealTimeProvider{},
	}
	svc.players[selfPK] = NewPlayer(selfPK)
	return svc
}

// fakeEnvelope encodes a payload and returns a decoded Envelope for testing.
func fakeEnvelope(t *testing.T, msgType string, payload interface{}) *Envelope {
	t.Helper()
	raw, err := EncodeMessage(msgType, payload)
	require.NoError(t, err)
	env, err := DecodeMessage(raw)
	require.NoError(t, err)
	return env
}
