package methane

import (
	"encoding/json"
	"fmt"
	"time"
)

// Message type constants for the Tox messaging protocol.
const (
	// MsgTypeLobbyAd is the type for lobby advertisement messages.
	MsgTypeLobbyAd = "lobby_ad"
	// MsgTypeMatchFound is the type for match-found notifications.
	MsgTypeMatchFound = "match_found"
	// MsgTypeReadyCheck is the type for ready-check requests.
	MsgTypeReadyCheck = "ready_check"
	// MsgTypeReadyResponse is the type for ready-check responses.
	MsgTypeReadyResponse = "ready_response"
	// MsgTypeGameLaunchInfo is the type for game launch information.
	MsgTypeGameLaunchInfo = "game_launch_info"
	// MsgTypeGameResult is the type for game result reporting.
	MsgTypeGameResult = "game_result"
	// MsgTypeLobbyList is the type for requesting the lobby list.
	MsgTypeLobbyList = "lobby_list"
)

// Envelope is the outer wrapper for all Tox protocol messages.
type Envelope struct {
	// Type identifies the message payload type (use MsgType* constants).
	Type string `json:"type"`
	// Payload is the JSON-encoded message body.
	Payload json.RawMessage `json:"payload"`
}

// LobbyAdvertisement is broadcast to friends to advertise an open lobby.
type LobbyAdvertisement struct {
	// LobbyID is the Tox conference ID.
	LobbyID uint32 `json:"lobby_id"`
	// HostPK is the hex-encoded public key of the lobby host.
	HostPK string `json:"host_pk"`
	// GameName is the name of the game being hosted.
	GameName string `json:"game_name"`
	// MapName is the name of the map or level.
	MapName string `json:"map_name"`
	// GameMode is the game mode of the lobby.
	GameMode GameMode `json:"game_mode"`
	// MaxPlayers is the maximum number of players allowed.
	MaxPlayers int `json:"max_players"`
	// CurPlayers is the current number of players.
	CurPlayers int `json:"cur_players"`
	// Region is the geographic region of the lobby.
	Region string `json:"region"`
	// Private indicates that the lobby is invite-only.
	Private bool `json:"private"`
}

// MatchFoundMessage notifies players that a match has been formed.
type MatchFoundMessage struct {
	// SessionID is the unique identifier for the new game session.
	SessionID string `json:"session_id"`
	// Players is the list of hex-encoded public keys of matched players.
	Players []string `json:"players"`
	// GameMode is the game mode for the session.
	GameMode GameMode `json:"game_mode"`
}

// ReadyCheckRequest asks players to confirm readiness within a timeout.
type ReadyCheckRequest struct {
	// SessionID is the session requiring confirmation.
	SessionID string `json:"session_id"`
	// Timeout is the duration players have to respond.
	// It is encoded on the wire as a Go duration string (e.g. "30s") for
	// readability and cross-implementation compatibility.
	Timeout time.Duration `json:"-"`
}

// readyCheckRequestWire is the on-wire JSON representation.
type readyCheckRequestWire struct {
	SessionID string `json:"session_id"`
	Timeout   string `json:"timeout"`
}

// MarshalJSON encodes Timeout as a human-readable duration string (e.g. "30s").
func (r ReadyCheckRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(readyCheckRequestWire{
		SessionID: r.SessionID,
		Timeout:   r.Timeout.String(),
	})
}

// UnmarshalJSON decodes a duration string (e.g. "30s") into Timeout.
func (r *ReadyCheckRequest) UnmarshalJSON(data []byte) error {
	var w readyCheckRequestWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	d, err := time.ParseDuration(w.Timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout duration %q: %w", w.Timeout, err)
	}
	r.SessionID = w.SessionID
	r.Timeout = d
	return nil
}

// ReadyCheckResponse is a player's response to a ready-check request.
type ReadyCheckResponse struct {
	// SessionID is the session being confirmed.
	SessionID string `json:"session_id"`
	// PlayerPK is the hex-encoded public key of the responding player.
	PlayerPK string `json:"player_pk"`
	// Ready indicates whether the player is ready.
	Ready bool `json:"ready"`
}

// GameLaunchInfo contains all information needed to start the game client.
type GameLaunchInfo struct {
	// SessionID is the associated game session.
	SessionID string `json:"session_id"`
	// HostAddr is the network address of the game host.
	HostAddr string `json:"host_addr"`
	// Port is the game server port.
	Port uint16 `json:"port"`
	// Args is the list of additional command-line arguments.
	Args []string `json:"args"`
	// Env is a map of environment variables to set.
	Env map[string]string `json:"env"`
}

// GameResult reports the outcome of a session for all participants.
type GameResult struct {
	// SessionID is the completed session.
	SessionID string `json:"session_id"`
	// Results maps each player's hex-encoded public key to their outcome.
	Results map[string]Outcome `json:"results"`
}

// EncodeMessage encodes a typed message payload into a JSON string ready for
// transmission over a Tox friend or conference message channel.
func EncodeMessage(msgType string, payload interface{}) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode message payload: %w", err)
	}
	env := Envelope{
		Type:    msgType,
		Payload: json.RawMessage(payloadBytes),
	}
	envBytes, err := json.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("encode message envelope: %w", err)
	}
	return string(envBytes), nil
}

// DecodeMessage parses a JSON string received via Tox into an Envelope.
func DecodeMessage(data string) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return nil, fmt.Errorf("decode message: %w", err)
	}
	return &env, nil
}

// DecodePayload unmarshals the Envelope's payload into target.
func DecodePayload(env *Envelope, target interface{}) error {
	if err := json.Unmarshal(env.Payload, target); err != nil {
		return fmt.Errorf("decode payload (type=%s): %w", env.Type, err)
	}
	return nil
}
