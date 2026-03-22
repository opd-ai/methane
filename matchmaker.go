package methane

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/opd-ai/toxcore"
	"github.com/sirupsen/logrus"
)

// MatchmakingService is the top-level decentralized matchmaking service.
// It manages lobbies, a matchmaking queue, and game sessions using a Tox
// instance for all peer-to-peer communication.
//
//export MatchmakingService
type MatchmakingService struct {
	tox                *toxcore.Tox
	lobbies            map[uint32]*Lobby
	sessions           map[string]*GameSession
	queue              *MatchmakingQueue
	players            map[[32]byte]*Player
	selfPK             [32]byte
	logger             *logrus.Logger
	mu                 sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
	tp                 TimeProvider
	autoAcceptFriends  bool // when true, incoming friend requests are accepted automatically

	// Callbacks
	onMatchFound func(*MatchFoundEvent)
	onLobbyFound func(*LobbyAdvertisement)
}

// NewMatchmakingService creates a MatchmakingService backed by the given Tox instance.
//
//export NewMatchmakingService
func NewMatchmakingService(tox *toxcore.Tox) *MatchmakingService {
	ctx, cancel := context.WithCancel(context.Background())
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	svc := &MatchmakingService{
		tox:      tox,
		lobbies:  make(map[uint32]*Lobby),
		sessions: make(map[string]*GameSession),
		queue:    NewMatchmakingQueue(DefaultQueueConfig()),
		players:  make(map[[32]byte]*Player),
		selfPK:   tox.GetSelfPublicKey(),
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
		tp:       RealTimeProvider{},
	}

	// Self-register.
	self := NewPlayerFromTox(tox)
	svc.players[self.PublicKey] = self

	return svc
}

// SetAutoAcceptFriends controls whether the service automatically accepts
// every incoming Tox friend request. It is disabled by default; callers should
// enable it only when they trust the discovery environment (e.g. a closed LAN
// lobby). When disabled, friend requests are silently ignored and applications
// must call tox.AddFriendByPublicKey themselves.
func (s *MatchmakingService) SetAutoAcceptFriends(accept bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoAcceptFriends = accept
}

// SetTimeProvider injects a TimeProvider for deterministic testing.
// Passing nil resets to the real wall-clock provider.
func (s *MatchmakingService) SetTimeProvider(tp TimeProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tp == nil {
		tp = RealTimeProvider{}
	}
	s.tp = tp
	s.queue.SetTimeProvider(tp)
}

// OnMatchFound registers a callback fired when a match is found.
func (s *MatchmakingService) OnMatchFound(callback func(*MatchFoundEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onMatchFound = callback
}

// OnLobbyFound registers a callback fired when a lobby advertisement is received.
func (s *MatchmakingService) OnLobbyFound(callback func(*LobbyAdvertisement)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLobbyFound = callback
}

// Start registers Tox callbacks and begins the service event loop.
// It returns immediately; use Stop() or cancel the context to halt.
func (s *MatchmakingService) Start(ctx context.Context) error {
	// Override internal context with the provided one.
	innerCtx, innerCancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel()
	s.ctx = innerCtx
	s.cancel = innerCancel
	s.mu.Unlock()

	s.registerToxCallbacks()
	s.queue.OnMatchFound(s.handleMatchFormed)

	go s.runLoop()
	s.logger.WithField("self_pk", hex.EncodeToString(s.selfPK[:])).Info("methane service started")
	return nil
}

// Stop shuts down the service event loop.
func (s *MatchmakingService) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	cancel()
	s.logger.Info("methane service stopped")
}

// runLoop is the main service loop; it drives the Tox event pump and queue.
func (s *MatchmakingService) runLoop() {
	s.mu.RLock()
	ctx := s.ctx
	s.mu.RUnlock()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		s.tox.Iterate()
		s.queue.RunOnce()
		interval := s.tox.IterationInterval()
		if interval <= 0 {
			interval = 50 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// registerToxCallbacks sets up all Tox event handlers.
func (s *MatchmakingService) registerToxCallbacks() {
	s.tox.OnFriendRequest(func(publicKey [32]byte, message string) {
		s.mu.RLock()
		accept := s.autoAcceptFriends
		s.mu.RUnlock()
		if !accept {
			s.logger.WithField("pk", hex.EncodeToString(publicKey[:])).Debug("friend request received; auto-accept disabled – ignoring")
			return
		}
		s.logger.WithField("pk", hex.EncodeToString(publicKey[:])).Debug("friend request received; auto-accepting")
		if _, err := s.tox.AddFriendByPublicKey(publicKey); err != nil {
			s.logger.WithError(err).Warn("auto-accept friend request failed")
		}
	})

	s.tox.OnFriendMessage(func(friendID uint32, message string) {
		s.handleFriendMessage(friendID, message)
	})

	s.tox.OnFriendConnectionStatus(func(friendID uint32, status toxcore.ConnectionStatus) {
		s.logger.WithFields(logrus.Fields{
			"friend_id": friendID,
			"status":    status,
		}).Debug("friend connection status changed")
	})
}

// handleFriendMessage dispatches an incoming friend message to the correct handler.
func (s *MatchmakingService) handleFriendMessage(friendID uint32, message string) {
	env, err := DecodeMessage(message)
	if err != nil {
		// Not a methane protocol message; ignore.
		return
	}
	switch env.Type {
	case MsgTypeLobbyAd:
		s.handleLobbyAd(env)
	case MsgTypeMatchFound:
		s.handleMatchFound(env)
	case MsgTypeReadyCheck:
		s.handleReadyCheck(friendID, env)
	case MsgTypeReadyResponse:
		s.handleReadyResponse(env)
	case MsgTypeGameResult:
		s.handleGameResult(env)
	case MsgTypeLobbyList:
		s.handleLobbyListRequest(friendID)
	default:
		s.logger.WithField("type", env.Type).Debug("unknown message type")
	}
}

// handleLobbyAd processes a received lobby advertisement.
func (s *MatchmakingService) handleLobbyAd(env *Envelope) {
	var ad LobbyAdvertisement
	if err := DecodePayload(env, &ad); err != nil {
		s.logger.WithError(err).Warn("failed to decode lobby ad")
		return
	}
	s.mu.RLock()
	cb := s.onLobbyFound
	s.mu.RUnlock()
	if cb != nil {
		cb(&ad)
	}
}

// handleMatchFound processes a received match-found notification.
// It creates a local GameSession record so that subsequent ready-check and
// game-result messages can be correlated to this session.
func (s *MatchmakingService) handleMatchFound(env *Envelope) {
	var msg MatchFoundMessage
	if err := DecodePayload(env, &msg); err != nil {
		s.logger.WithError(err).Warn("failed to decode match found")
		return
	}
	players := make([]*Player, 0, len(msg.Players))
	for _, pkHex := range msg.Players {
		pkBytes, err := hex.DecodeString(pkHex)
		if err != nil || len(pkBytes) != 32 {
			continue
		}
		var pk [32]byte
		copy(pk[:], pkBytes)
		players = append(players, s.GetOrCreatePlayer(pk))
	}

	// Create (or reuse) a session record so that ready-check and result
	// messages for this session can be processed by this node.
	// Read tp and check existence without holding the write lock to keep
	// the critical section short.
	s.mu.RLock()
	tp := s.tp
	_, exists := s.sessions[msg.SessionID]
	s.mu.RUnlock()

	var cb func(*MatchFoundEvent)
	if !exists {
		session := newGameSessionWithID(msg.SessionID, players, msg.GameMode, tp)
		s.mu.Lock()
		// Re-check under write lock to guard against a concurrent insert.
		if _, alreadyInserted := s.sessions[msg.SessionID]; !alreadyInserted {
			s.sessions[msg.SessionID] = session
		}
		cb = s.onMatchFound
		s.mu.Unlock()
	} else {
		s.mu.RLock()
		cb = s.onMatchFound
		s.mu.RUnlock()
	}

	if cb != nil {
		cb(&MatchFoundEvent{SessionID: msg.SessionID, Players: players})
	}
}

// handleReadyCheck processes a ready-check request.
func (s *MatchmakingService) handleReadyCheck(friendID uint32, env *Envelope) {
	var req ReadyCheckRequest
	if err := DecodePayload(env, &req); err != nil {
		s.logger.WithError(err).Warn("failed to decode ready check")
		return
	}
	s.mu.RLock()
	session, ok := s.sessions[req.SessionID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	session.StartReadyCheck(req.Timeout)

	// Auto-respond ready.
	resp := ReadyCheckResponse{
		SessionID: req.SessionID,
		PlayerPK:  hex.EncodeToString(s.selfPK[:]),
		Ready:     true,
	}
	msg, err := EncodeMessage(MsgTypeReadyResponse, resp)
	if err != nil {
		s.logger.WithError(err).Warn("failed to encode ready response")
		return
	}
	if err := s.tox.SendFriendMessage(friendID, msg); err != nil {
		s.logger.WithError(err).Warn("failed to send ready response")
	}
}

// handleReadyResponse processes a ready-check response from a peer.
func (s *MatchmakingService) handleReadyResponse(env *Envelope) {
	var resp ReadyCheckResponse
	if err := DecodePayload(env, &resp); err != nil {
		s.logger.WithError(err).Warn("failed to decode ready response")
		return
	}
	s.mu.RLock()
	session, ok := s.sessions[resp.SessionID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	pkBytes, err := hex.DecodeString(resp.PlayerPK)
	if err != nil || len(pkBytes) != 32 {
		return
	}
	var pk [32]byte
	copy(pk[:], pkBytes)
	session.RecordReadyAck(pk, resp.Ready)
}

// handleGameResult processes a game result message.
func (s *MatchmakingService) handleGameResult(env *Envelope) {
	var result GameResult
	if err := DecodePayload(env, &result); err != nil {
		s.logger.WithError(err).Warn("failed to decode game result")
		return
	}
	s.mu.RLock()
	session, ok := s.sessions[result.SessionID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	if err := session.RecordResult(&result); err != nil {
		s.logger.WithError(err).Warn("failed to record game result")
		return
	}
	session.Complete()
}

// handleLobbyListRequest sends the list of non-private lobbies to a friend.
func (s *MatchmakingService) handleLobbyListRequest(friendID uint32) {
	s.mu.RLock()
	lobbies := make([]*LobbyAdvertisement, 0, len(s.lobbies))
	for _, l := range s.lobbies {
		if !l.Config.Private {
			lobbies = append(lobbies, l.ToAdvertisement())
		}
	}
	s.mu.RUnlock()

	for _, ad := range lobbies {
		msg, err := EncodeMessage(MsgTypeLobbyAd, ad)
		if err != nil {
			continue
		}
		if err := s.tox.SendFriendMessage(friendID, msg); err != nil {
			s.logger.WithError(err).Warn("failed to send lobby ad")
		}
	}
}

// handleMatchFormed is called by the queue when a group of players have been matched.
func (s *MatchmakingService) handleMatchFormed(players []*Player) {
	mode := ModeFFA
	if len(players) > 0 {
		mode = players[0].Preferences.GameMode
	}
	session := NewGameSession(players, mode)
	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()

	s.logger.WithFields(logrus.Fields{
		"session_id": session.ID,
		"players":    len(players),
	}).Info("match formed")

	// Notify all players via friend messages.
	playerPKs := make([]string, len(players))
	for i, p := range players {
		playerPKs[i] = p.PublicKeyHex()
	}
	msg, err := EncodeMessage(MsgTypeMatchFound, MatchFoundMessage{
		SessionID: session.ID,
		Players:   playerPKs,
		GameMode:  session.GameMode,
	})
	if err != nil {
		s.logger.WithError(err).Warn("failed to encode match found message")
		return
	}

	// Notify players via friend messages (only if tox is available).
	if s.tox != nil {
		friends := s.tox.GetFriends()
		for fID, friend := range friends {
			for _, p := range players {
				if friend.PublicKey == p.PublicKey {
					if err := s.tox.SendFriendMessage(fID, msg); err != nil {
						s.logger.WithError(err).Warn("failed to send match found")
					}
					break
				}
			}
		}
	}

	// Fire local callback.
	s.mu.RLock()
	cb := s.onMatchFound
	s.mu.RUnlock()
	if cb != nil {
		cb(&MatchFoundEvent{SessionID: session.ID, Players: players})
	}
}

// CreateLobby creates a new Tox conference-backed lobby.
func (s *MatchmakingService) CreateLobby(config LobbyConfig) (*Lobby, error) {
	confID, err := s.tox.ConferenceNew()
	if err != nil {
		return nil, fmt.Errorf("create lobby conference: %w", err)
	}
	s.mu.Lock()
	lobby := NewLobby(confID, s.selfPK, config, s.tp)
	self := s.players[s.selfPK]
	if self != nil {
		_ = lobby.AddPlayer(self)
	}
	s.lobbies[confID] = lobby
	s.mu.Unlock()

	s.logger.WithFields(logrus.Fields{
		"lobby_id":  confID,
		"game_name": config.GameName,
	}).Info("lobby created")

	return lobby, nil
}

// JoinLobby asks hostFriendID to invite us into the conference lobbyID.
func (s *MatchmakingService) JoinLobby(lobbyID uint32, hostFriendID uint32) error {
	if err := s.tox.ConferenceInvite(hostFriendID, lobbyID); err != nil {
		return fmt.Errorf("join lobby %d: %w", lobbyID, err)
	}
	return nil
}

// LeaveLobby removes the local player from the given lobby.
func (s *MatchmakingService) LeaveLobby(lobbyID uint32) error {
	s.mu.Lock()
	lobby, ok := s.lobbies[lobbyID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("lobby %d not found", lobbyID)
	}
	lobby.RemovePlayer(s.selfPK)
	if lobby.PlayerCount() == 0 {
		delete(s.lobbies, lobbyID)
	}
	s.mu.Unlock()
	return nil
}

// ListLobbies returns all known lobbies.
func (s *MatchmakingService) ListLobbies() []*Lobby {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Lobby, 0, len(s.lobbies))
	for _, l := range s.lobbies {
		out = append(out, l)
	}
	return out
}

// EnqueueForMatch adds the local player to the matchmaking queue for the given
// mode and region. It also updates the player's stored preferences so that
// handleMatchFormed can correctly record the session's GameMode.
func (s *MatchmakingService) EnqueueForMatch(mode GameMode, region string) error {
	self := s.GetSelfPlayer()
	self.mu.Lock()
	self.Preferences.GameMode = mode
	self.Preferences.Region = region
	self.mu.Unlock()
	return s.queue.Enqueue(self, mode, region)
}

// LeaveQueue removes the local player from the matchmaking queue.
func (s *MatchmakingService) LeaveQueue() bool {
	return s.queue.Dequeue(s.selfPK)
}

// GetOrCreatePlayer returns an existing Player record for pk, or creates one.
func (s *MatchmakingService) GetOrCreatePlayer(pk [32]byte) *Player {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.players[pk]; ok {
		return p
	}
	p := NewPlayer(pk)
	s.players[pk] = p
	return p
}

// GetSelfPlayer returns the Player record for the local Tox identity.
func (s *MatchmakingService) GetSelfPlayer() *Player {
	return s.GetOrCreatePlayer(s.selfPK)
}

// AdvertiseLobby sends a LobbyAd message to all online friends.
func (s *MatchmakingService) AdvertiseLobby(lobby *Lobby) {
	if s.tox == nil {
		return
	}
	ad := lobby.ToAdvertisement()
	msg, err := EncodeMessage(MsgTypeLobbyAd, ad)
	if err != nil {
		s.logger.WithError(err).Warn("failed to encode lobby advertisement")
		return
	}
	friends := s.tox.GetFriends()
	for fID := range friends {
		if err := s.tox.SendFriendMessage(fID, msg); err != nil {
			s.logger.WithFields(logrus.Fields{
				"friend_id": fID,
			}).WithError(err).Debug("failed to send lobby ad to friend")
		}
	}
}

// RequestLobbyList asks a friend to send their known lobby list.
func (s *MatchmakingService) RequestLobbyList(friendID uint32) error {
	msg, err := EncodeMessage(MsgTypeLobbyList, struct{}{})
	if err != nil {
		return fmt.Errorf("encode lobby list request: %w", err)
	}
	if err := s.tox.SendFriendMessage(friendID, msg); err != nil {
		return fmt.Errorf("send lobby list request: %w", err)
	}
	return nil
}
