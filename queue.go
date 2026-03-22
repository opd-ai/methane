package methane

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// QueueConfig holds configuration for the matchmaking queue.
type QueueConfig struct {
	// MaxRatingDiff is the initial maximum rating difference allowed for a match.
	MaxRatingDiff float64
	// QueueTimeout is how long a player can wait before the rating window widens.
	QueueTimeout time.Duration
	// MatchSize is the number of players required to form a match.
	MatchSize int
	// RatingWindow is the initial rating window (same as MaxRatingDiff).
	RatingWindow float64
	// WindowGrowth is how much the rating window grows per QueueTimeout elapsed.
	WindowGrowth float64
}

// DefaultQueueConfig returns a sensible default QueueConfig.
func DefaultQueueConfig() *QueueConfig {
	return &QueueConfig{
		MaxRatingDiff: 300.0,
		QueueTimeout:  30 * time.Second,
		MatchSize:     2,
		RatingWindow:  300.0,
		WindowGrowth:  50.0,
	}
}

// QueueEntry holds a single player's position in the matchmaking queue.
type QueueEntry struct {
	// Player is the queued player.
	Player *Player
	// GameMode is the game mode the player is queuing for.
	GameMode GameMode
	// Region is the geographic region preference.
	Region string
	// EnqueuedAt is the time the player joined the queue.
	EnqueuedAt time.Time
	// ratingWindow is the current effective rating window for this entry.
	ratingWindow float64
}

// MatchmakingQueue manages the pool of queued players and forms matches.
type MatchmakingQueue struct {
	entries []*QueueEntry
	config  *QueueConfig
	onMatch func([]*Player)
	mu      sync.RWMutex
	tp      TimeProvider
}

// NewMatchmakingQueue creates a new MatchmakingQueue with the given config.
//
//export NewMatchmakingQueue
func NewMatchmakingQueue(config *QueueConfig) *MatchmakingQueue {
	if config == nil {
		config = DefaultQueueConfig()
	}
	return &MatchmakingQueue{
		config: config,
		tp:     RealTimeProvider{},
	}
}

// SetTimeProvider injects a TimeProvider for deterministic testing.
// Passing nil resets to the real wall-clock provider.
func (q *MatchmakingQueue) SetTimeProvider(tp TimeProvider) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if tp == nil {
		q.tp = RealTimeProvider{}
		return
	}
	q.tp = tp
}

// OnMatchFound registers a callback invoked when a match is formed.
func (q *MatchmakingQueue) OnMatchFound(callback func([]*Player)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.onMatch = callback
}

// Enqueue adds a player to the matchmaking queue.
func (q *MatchmakingQueue) Enqueue(p *Player, mode GameMode, region string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, e := range q.entries {
		if e.Player.PublicKey == p.PublicKey {
			return fmt.Errorf("player %s is already in queue", p.PublicKeyHex())
		}
	}
	q.entries = append(q.entries, &QueueEntry{
		Player:       p,
		GameMode:     mode,
		Region:       region,
		EnqueuedAt:   q.tp.Now(),
		ratingWindow: q.config.RatingWindow,
	})
	return nil
}

// Dequeue removes the player with the given public key from the queue.
// Returns true if the player was found and removed.
func (q *MatchmakingQueue) Dequeue(pk [32]byte) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, e := range q.entries {
		if e.Player.PublicKey == pk {
			q.entries = append(q.entries[:i], q.entries[i+1:]...)
			return true
		}
	}
	return false
}

// Size returns the number of players currently in the queue.
func (q *MatchmakingQueue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.entries)
}

// RunOnce attempts to form matches from the current queue contents.
// It updates rating windows based on time elapsed, then calls findMatches.
func (q *MatchmakingQueue) RunOnce() {
	q.mu.Lock()
	now := q.tp.Now()
	// Grow rating windows for long-waiting entries.
	for _, e := range q.entries {
		elapsed := now.Sub(e.EnqueuedAt)
		if q.config.QueueTimeout > 0 {
			windows := int(elapsed / q.config.QueueTimeout)
			e.ratingWindow = q.config.RatingWindow + float64(windows)*q.config.WindowGrowth
		} else {
			// QueueTimeout of zero means no window growth; keep the initial window.
			e.ratingWindow = q.config.RatingWindow
		}
	}
	groups := q.findMatches()
	// Remove matched entries.
	matchedKeys := make(map[[32]byte]bool)
	for _, group := range groups {
		for _, e := range group {
			matchedKeys[e.Player.PublicKey] = true
		}
	}
	remaining := q.entries[:0]
	for _, e := range q.entries {
		if !matchedKeys[e.Player.PublicKey] {
			remaining = append(remaining, e)
		}
	}
	q.entries = remaining
	cb := q.onMatch
	q.mu.Unlock()

	// Fire callbacks outside the lock.
	if cb != nil {
		for _, group := range groups {
			players := make([]*Player, len(group))
			for i, e := range group {
				players[i] = e.Player
			}
			cb(players)
		}
	}
}

// findMatches returns groups of QueueEntry slices that form valid matches.
// Must be called with q.mu held.
func (q *MatchmakingQueue) findMatches() [][]*QueueEntry {
	// Group by GameMode.
	byMode := make(map[GameMode][]*QueueEntry)
	for _, e := range q.entries {
		byMode[e.GameMode] = append(byMode[e.GameMode], e)
	}

	var result [][]*QueueEntry
	used := make(map[*QueueEntry]bool)

	for _, modeEntries := range byMode {
		// Sort by rating.
		sort.Slice(modeEntries, func(i, j int) bool {
			return modeEntries[i].Player.Rating < modeEntries[j].Player.Rating
		})

		n := q.config.MatchSize
		for i := 0; i <= len(modeEntries)-n; i++ {
			anchor := modeEntries[i]
			if used[anchor] {
				continue
			}
			// Try to find n players within the anchor's rating window.
			group := []*QueueEntry{anchor}
			for j := i + 1; j < len(modeEntries) && len(group) < n; j++ {
				candidate := modeEntries[j]
				if used[candidate] {
					continue
				}
				if !ratingsCompatible(anchor, candidate) {
					break // sorted, so no further candidate can match
				}
				if !regionsCompatible(anchor, candidate) {
					continue
				}
				group = append(group, candidate)
			}
			if len(group) == n {
				result = append(result, group)
				for _, e := range group {
					used[e] = true
				}
			}
		}
	}
	return result
}

// ratingsCompatible returns true if two entries are within each other's window.
func ratingsCompatible(a, b *QueueEntry) bool {
	diff := math.Abs(a.Player.Rating - b.Player.Rating)
	window := math.Min(a.ratingWindow, b.ratingWindow)
	return diff <= window
}

// regionsCompatible returns true if two entries can be matched based on region.
// Entries that have waited long enough (window grown) accept any region.
func regionsCompatible(a, b *QueueEntry) bool {
	if a.Region == RegionAny || b.Region == RegionAny {
		return true
	}
	if a.Region == b.Region {
		return true
	}
	// Allow cross-region if the window has grown (player has waited a full timeout).
	aRelaxed := a.ratingWindow > a.Player.Preferences.MaxRatingDiff
	bRelaxed := b.ratingWindow > b.Player.Preferences.MaxRatingDiff
	return aRelaxed || bRelaxed
}
