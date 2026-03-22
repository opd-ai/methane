package methane

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestLobby(maxPlayers int) *Lobby {
	var pk [32]byte
	pk[0] = 1
	cfg := LobbyConfig{
		GameName:   "TestGame",
		MaxPlayers: maxPlayers,
		Region:     RegionAny,
	}
	tp := NewMockTimeProvider(time.Now())
	return NewLobby(42, pk, cfg, tp)
}

func TestLobby_IsHost(t *testing.T) {
	var hostPK [32]byte
	hostPK[0] = 1
	lobby := makeTestLobby(4)
	assert.True(t, lobby.IsHost(hostPK))

	var otherPK [32]byte
	otherPK[0] = 2
	assert.False(t, lobby.IsHost(otherPK))
}

func TestLobby_IsFull_Empty(t *testing.T) {
	lobby := makeTestLobby(2)
	assert.False(t, lobby.IsFull())
}

func TestLobby_AddPlayer_Success(t *testing.T) {
	lobby := makeTestLobby(4)
	var pk [32]byte
	pk[0] = 5
	p := NewPlayer(pk)
	err := lobby.AddPlayer(p)
	require.NoError(t, err)
	assert.Equal(t, 1, lobby.PlayerCount())
}

func TestLobby_AddPlayer_Duplicate(t *testing.T) {
	lobby := makeTestLobby(4)
	var pk [32]byte
	pk[0] = 5
	p := NewPlayer(pk)
	require.NoError(t, lobby.AddPlayer(p))
	err := lobby.AddPlayer(p)
	assert.Error(t, err, "adding same player twice should fail")
}

func TestLobby_AddPlayer_Full(t *testing.T) {
	lobby := makeTestLobby(1)
	var pk1 [32]byte
	pk1[0] = 5
	require.NoError(t, lobby.AddPlayer(NewPlayer(pk1)))

	var pk2 [32]byte
	pk2[0] = 6
	err := lobby.AddPlayer(NewPlayer(pk2))
	assert.Error(t, err, "adding player to full lobby should fail")
}

func TestLobby_RemovePlayer(t *testing.T) {
	lobby := makeTestLobby(4)
	var pk [32]byte
	pk[0] = 7
	p := NewPlayer(pk)
	require.NoError(t, lobby.AddPlayer(p))
	assert.Equal(t, 1, lobby.PlayerCount())
	lobby.RemovePlayer(pk)
	assert.Equal(t, 0, lobby.PlayerCount())
}

func TestLobby_RemovePlayer_NotPresent(t *testing.T) {
	lobby := makeTestLobby(4)
	var pk [32]byte
	// Removing a non-existent player should not panic.
	lobby.RemovePlayer(pk)
	assert.Equal(t, 0, lobby.PlayerCount())
}

func TestLobby_ToAdvertisement(t *testing.T) {
	lobby := makeTestLobby(4)
	var pk [32]byte
	pk[0] = 8
	require.NoError(t, lobby.AddPlayer(NewPlayer(pk)))

	ad := lobby.ToAdvertisement()
	assert.Equal(t, uint32(42), ad.LobbyID)
	assert.Equal(t, "TestGame", ad.GameName)
	assert.Equal(t, 4, ad.MaxPlayers)
	assert.Equal(t, 1, ad.CurPlayers)
	assert.Equal(t, RegionAny, ad.Region)
}

func TestLobby_IsFull_AfterFilling(t *testing.T) {
	lobby := makeTestLobby(2)
	var pk1, pk2 [32]byte
	pk1[0] = 10
	pk2[0] = 11
	require.NoError(t, lobby.AddPlayer(NewPlayer(pk1)))
	require.NoError(t, lobby.AddPlayer(NewPlayer(pk2)))
	assert.True(t, lobby.IsFull())
}
