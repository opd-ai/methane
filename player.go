package methane

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/opd-ai/toxcore"
)

// Outcome represents the result of a match for a single player.
type Outcome float64

const (
	// OutcomeLoss represents a loss (score 0.0).
	OutcomeLoss Outcome = 0.0
	// OutcomeDraw represents a draw (score 0.5).
	OutcomeDraw Outcome = 0.5
	// OutcomeWin represents a win (score 1.0).
	OutcomeWin Outcome = 1.0
)

// MatchResult records the outcome of a single game for a player.
type MatchResult struct {
	// SessionID is the unique identifier of the session.
	SessionID string `json:"session_id"`
	// Outcome is the result of the match.
	Outcome Outcome `json:"outcome"`
	// OppRating is the average opponent rating at the time of the match.
	OppRating float64 `json:"opp_rating"`
	// PlayedAt is the time the match was recorded.
	PlayedAt time.Time `json:"played_at"`
}

// Preferences holds a player's matchmaking preferences.
type Preferences struct {
	// GameMode is the preferred game mode.
	GameMode GameMode `json:"game_mode"`
	// Region is the preferred region (use Region* constants).
	Region string `json:"region"`
	// MaxRatingDiff is the maximum acceptable rating difference.
	MaxRatingDiff float64 `json:"max_rating_diff"`
}

// Player represents a participant in matchmaking.
//
//export Player
type Player struct {
	// PublicKey is the Tox public key identifying this player.
	PublicKey [32]byte `json:"public_key"`
	// DisplayName is the human-readable name.
	DisplayName string `json:"display_name"`
	// Rating is the Glicko-2 rating (default 1500).
	Rating float64 `json:"rating"`
	// RatingDev is the Glicko-2 rating deviation (default 350).
	RatingDev float64 `json:"rating_dev"`
	// Volatility is the Glicko-2 volatility (default 0.06).
	Volatility float64 `json:"volatility"`
	// Wins is the total number of wins.
	Wins int `json:"wins"`
	// Losses is the total number of losses.
	Losses int `json:"losses"`
	// Draws is the total number of draws.
	Draws int `json:"draws"`
	// History is the list of past match results.
	History []MatchResult `json:"history"`
	// Preferences holds the player's matchmaking preferences.
	Preferences Preferences `json:"preferences"`
	mu          sync.RWMutex
}

// NewPlayer creates a new Player with the given public key and default Glicko-2 values.
//
//export NewPlayer
func NewPlayer(publicKey [32]byte) *Player {
	return &Player{
		PublicKey:  publicKey,
		Rating:     1500.0,
		RatingDev:  350.0,
		Volatility: 0.06,
		Preferences: Preferences{
			GameMode:      ModeFFA,
			Region:        RegionAny,
			MaxRatingDiff: 300.0,
		},
	}
}

// NewPlayerFromTox creates a Player using the self public key from a Tox instance.
//
//export NewPlayerFromTox
func NewPlayerFromTox(tox *toxcore.Tox) *Player {
	pk := tox.GetSelfPublicKey()
	p := NewPlayer(pk)
	p.DisplayName = tox.SelfGetName()
	return p
}

// PublicKeyHex returns the player's public key as a hex string.
func (p *Player) PublicKeyHex() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return hex.EncodeToString(p.PublicKey[:])
}

// GetRating returns the player's current Glicko-2 rating.
func (p *Player) GetRating() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Rating
}

// UpdateRating updates the player's Glicko-2 rating parameters atomically.
func (p *Player) UpdateRating(r, rd, v float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Rating = r
	p.RatingDev = rd
	p.Volatility = v
}

// RecordResult appends a match result and updates win/loss/draw counters.
func (p *Player) RecordResult(result MatchResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.History = append(p.History, result)
	switch result.Outcome {
	case OutcomeWin:
		p.Wins++
	case OutcomeLoss:
		p.Losses++
	case OutcomeDraw:
		p.Draws++
	}
}

// playerJSON is a helper for JSON marshalling that avoids exposing the mutex.
type playerJSON struct {
	PublicKey   string        `json:"public_key"`
	DisplayName string        `json:"display_name"`
	Rating      float64       `json:"rating"`
	RatingDev   float64       `json:"rating_dev"`
	Volatility  float64       `json:"volatility"`
	Wins        int           `json:"wins"`
	Losses      int           `json:"losses"`
	Draws       int           `json:"draws"`
	History     []MatchResult `json:"history"`
	Preferences Preferences   `json:"preferences"`
}

// MarshalJSON implements json.Marshaler.
func (p *Player) MarshalJSON() ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return json.Marshal(playerJSON{
		PublicKey:   hex.EncodeToString(p.PublicKey[:]),
		DisplayName: p.DisplayName,
		Rating:      p.Rating,
		RatingDev:   p.RatingDev,
		Volatility:  p.Volatility,
		Wins:        p.Wins,
		Losses:      p.Losses,
		Draws:       p.Draws,
		History:     p.History,
		Preferences: p.Preferences,
	})
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *Player) UnmarshalJSON(data []byte) error {
	var pj playerJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return fmt.Errorf("player unmarshal: %w", err)
	}
	pkBytes, err := hex.DecodeString(pj.PublicKey)
	if err != nil {
		return fmt.Errorf("player unmarshal public key: %w", err)
	}
	if len(pkBytes) != 32 {
		return fmt.Errorf("player unmarshal: public key must be 32 bytes")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	copy(p.PublicKey[:], pkBytes)
	p.DisplayName = pj.DisplayName
	p.Rating = pj.Rating
	p.RatingDev = pj.RatingDev
	p.Volatility = pj.Volatility
	p.Wins = pj.Wins
	p.Losses = pj.Losses
	p.Draws = pj.Draws
	p.History = pj.History
	p.Preferences = pj.Preferences
	return nil
}
