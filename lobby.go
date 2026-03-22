package methane

import (
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// LobbyConfig holds the configuration for creating a new lobby.
type LobbyConfig struct {
	// GameName is the name of the game.
	GameName string
	// MapName is the name of the map or level.
	MapName string
	// GameMode is the game mode.
	GameMode GameMode
	// MaxPlayers is the maximum number of players in the lobby.
	MaxPlayers int
	// Region is the geographic region.
	Region string
	// Private indicates an invite-only lobby.
	Private bool
}

// Lobby represents an active game lobby backed by a Tox conference.
type Lobby struct {
	// ID is the Tox conference ID.
	ID uint32
	// Config holds the lobby configuration.
	Config LobbyConfig
	// HostPK is the public key of the lobby host.
	HostPK [32]byte
	// Players is the list of players currently in the lobby.
	Players []*Player
	// CreatedAt is the time the lobby was created.
	CreatedAt time.Time
	mu        sync.RWMutex
}

// NewLobby creates a Lobby with the given conference ID, host public key, and config.
func NewLobby(id uint32, hostPK [32]byte, config LobbyConfig, tp TimeProvider) *Lobby {
	if tp == nil {
		tp = RealTimeProvider{}
	}
	return &Lobby{
		ID:        id,
		Config:    config,
		HostPK:    hostPK,
		Players:   make([]*Player, 0, config.MaxPlayers),
		CreatedAt: tp.Now(),
	}
}

// IsHost returns true if pk matches the lobby's host public key.
func (l *Lobby) IsHost(pk [32]byte) bool {
	return l.HostPK == pk
}

// IsFull returns true if the lobby has reached its maximum player count.
func (l *Lobby) IsFull() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.Players) >= l.Config.MaxPlayers
}

// PlayerCount returns the current number of players in the lobby.
func (l *Lobby) PlayerCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.Players)
}

// AddPlayer adds a player to the lobby. Returns an error if the lobby is full
// or the player is already present.
func (l *Lobby) AddPlayer(p *Player) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.Players) >= l.Config.MaxPlayers {
		return fmt.Errorf("lobby %d is full (%d/%d)", l.ID, len(l.Players), l.Config.MaxPlayers)
	}
	for _, existing := range l.Players {
		if existing.PublicKey == p.PublicKey {
			return fmt.Errorf("player %s is already in lobby %d", hex.EncodeToString(p.PublicKey[:]), l.ID)
		}
	}
	l.Players = append(l.Players, p)
	return nil
}

// RemovePlayer removes the player with the given public key from the lobby.
func (l *Lobby) RemovePlayer(pk [32]byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	filtered := l.Players[:0]
	for _, p := range l.Players {
		if p.PublicKey != pk {
			filtered = append(filtered, p)
		}
	}
	l.Players = filtered
}

// ToAdvertisement converts the lobby to a LobbyAdvertisement for broadcasting.
func (l *Lobby) ToAdvertisement() *LobbyAdvertisement {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return &LobbyAdvertisement{
		LobbyID:    l.ID,
		HostPK:     hex.EncodeToString(l.HostPK[:]),
		GameName:   l.Config.GameName,
		MapName:    l.Config.MapName,
		GameMode:   l.Config.GameMode,
		MaxPlayers: l.Config.MaxPlayers,
		CurPlayers: len(l.Players),
		Region:     l.Config.Region,
		Private:    l.Config.Private,
	}
}
