package methane

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makePlayers(n int) []*Player {
	players := make([]*Player, n)
	for i := range players {
		var pk [32]byte
		pk[0] = byte(i + 1)
		players[i] = NewPlayer(pk)
	}
	return players
}

func TestNewGameSession_Fields(t *testing.T) {
	players := makePlayers(2)
	s := NewGameSession(players, ModeFFA)
	assert.NotEmpty(t, s.ID)
	assert.Equal(t, MatchFormed, s.State)
	assert.Len(t, s.Players, 2)
	assert.Len(t, s.ReadyAcks, 2)
	assert.Equal(t, ModeFFA, s.GameMode)
}

func TestSession_StartReadyCheck(t *testing.T) {
	s := NewGameSession(makePlayers(2), ModeFFA)
	s.StartReadyCheck(10 * time.Second)
	assert.Equal(t, ReadyCheck, s.State)
}

func TestSession_RecordReadyAck_AllReady(t *testing.T) {
	players := makePlayers(2)
	s := NewGameSession(players, ModeFFA)
	s.StartReadyCheck(10 * time.Second)

	assert.False(t, s.IsReadyCheckComplete())
	s.RecordReadyAck(players[0].PublicKey, true)
	assert.False(t, s.IsReadyCheckComplete())
	s.RecordReadyAck(players[1].PublicKey, true)
	assert.True(t, s.IsReadyCheckComplete())
	assert.True(t, s.AllPlayersReady())
}

func TestSession_RecordReadyAck_NotReady(t *testing.T) {
	players := makePlayers(2)
	s := NewGameSession(players, ModeFFA)
	s.StartReadyCheck(10 * time.Second)
	s.RecordReadyAck(players[0].PublicKey, false)
	s.RecordReadyAck(players[1].PublicKey, true)
	// Not all ready.
	assert.False(t, s.AllPlayersReady())
}

func TestSession_StartGame(t *testing.T) {
	s := NewGameSession(makePlayers(2), ModeFFA)
	info := &GameLaunchInfo{SessionID: s.ID, HostAddr: "127.0.0.1", Port: 7777}
	s.StartGame(info)
	assert.Equal(t, Launching, s.State)
}

func TestSession_RecordResult(t *testing.T) {
	players := makePlayers(2)
	s := NewGameSession(players, ModeFFA)
	result := &GameResult{
		SessionID: s.ID,
		Results: map[string]Outcome{
			players[0].PublicKeyHex(): OutcomeWin,
			players[1].PublicKeyHex(): OutcomeLoss,
		},
	}
	err := s.RecordResult(result)
	require.NoError(t, err)
	assert.Equal(t, InProgress, s.State)
	assert.Equal(t, OutcomeWin, s.Results[players[0].PublicKeyHex()])
}

func TestSession_RecordResult_WrongID(t *testing.T) {
	s := NewGameSession(makePlayers(2), ModeFFA)
	result := &GameResult{
		SessionID: "wrong-id",
		Results:   map[string]Outcome{},
	}
	err := s.RecordResult(result)
	assert.Error(t, err, "result with wrong session ID should fail")
}

func TestSession_Complete(t *testing.T) {
	s := NewGameSession(makePlayers(2), ModeFFA)
	s.Complete()
	assert.Equal(t, Completed, s.State)
}

func TestSession_Cancel(t *testing.T) {
	s := NewGameSession(makePlayers(2), ModeFFA)
	s.Cancel()
	assert.Equal(t, Cancelled, s.State)
}

func TestSession_SetTimeProvider(t *testing.T) {
	s := NewGameSession(makePlayers(2), ModeFFA)
	tp := NewMockTimeProvider(time.Now())
	// Should not panic.
	s.SetTimeProvider(tp)
}

func TestSession_EmptyReadyCheck(t *testing.T) {
	s := NewGameSession([]*Player{}, ModeFFA)
	// With no players, IsReadyCheckComplete returns false (len==0).
	assert.False(t, s.IsReadyCheckComplete())
}
