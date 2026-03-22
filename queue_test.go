package methane

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPlayer(rating float64) *Player {
	var pk [32]byte
	pk[0] = byte(int(rating) % 256)
	pk[1] = byte(int(rating) / 256)
	p := NewPlayer(pk)
	p.Rating = rating
	p.Preferences.MaxRatingDiff = 300.0
	return p
}

func TestEnqueue_Basic(t *testing.T) {
	q := NewMatchmakingQueue(DefaultQueueConfig())
	p := newTestPlayer(1500)
	err := q.Enqueue(p, ModeFFA, RegionAny)
	require.NoError(t, err)
	assert.Equal(t, 1, q.Size())
}

func TestEnqueue_Duplicate(t *testing.T) {
	q := NewMatchmakingQueue(DefaultQueueConfig())
	p := newTestPlayer(1500)
	require.NoError(t, q.Enqueue(p, ModeFFA, RegionAny))
	err := q.Enqueue(p, ModeFFA, RegionAny)
	assert.Error(t, err, "duplicate enqueue should fail")
}

func TestDequeue_Found(t *testing.T) {
	q := NewMatchmakingQueue(DefaultQueueConfig())
	p := newTestPlayer(1500)
	require.NoError(t, q.Enqueue(p, ModeFFA, RegionAny))
	ok := q.Dequeue(p.PublicKey)
	assert.True(t, ok)
	assert.Equal(t, 0, q.Size())
}

func TestDequeue_NotFound(t *testing.T) {
	q := NewMatchmakingQueue(DefaultQueueConfig())
	var pk [32]byte
	ok := q.Dequeue(pk)
	assert.False(t, ok)
}

func TestRunOnce_FormsMatch(t *testing.T) {
	cfg := DefaultQueueConfig()
	cfg.MatchSize = 2
	cfg.MaxRatingDiff = 300
	cfg.RatingWindow = 300

	q := NewMatchmakingQueue(cfg)
	matched := make(chan []*Player, 1)
	q.OnMatchFound(func(players []*Player) {
		matched <- players
	})

	p1 := newTestPlayer(1500)
	p2 := newTestPlayer(1550)
	require.NoError(t, q.Enqueue(p1, ModeFFA, RegionAny))
	require.NoError(t, q.Enqueue(p2, ModeFFA, RegionAny))

	q.RunOnce()

	select {
	case players := <-matched:
		assert.Len(t, players, 2)
	case <-time.After(time.Second):
		t.Fatal("expected match to be formed")
	}
	assert.Equal(t, 0, q.Size(), "queue should be empty after match")
}

func TestRunOnce_NoMatch_RatingTooFar(t *testing.T) {
	cfg := DefaultQueueConfig()
	cfg.MatchSize = 2
	cfg.MaxRatingDiff = 100
	cfg.RatingWindow = 100

	q := NewMatchmakingQueue(cfg)
	matched := false
	q.OnMatchFound(func(players []*Player) {
		matched = true
	})

	p1 := newTestPlayer(1000)
	p2 := newTestPlayer(1500)
	require.NoError(t, q.Enqueue(p1, ModeFFA, RegionAny))
	require.NoError(t, q.Enqueue(p2, ModeFFA, RegionAny))

	q.RunOnce()
	assert.False(t, matched, "should not match players with large rating gap")
	assert.Equal(t, 2, q.Size())
}

func TestRunOnce_RatingWindowGrows(t *testing.T) {
	cfg := DefaultQueueConfig()
	cfg.MatchSize = 2
	cfg.MaxRatingDiff = 100
	cfg.RatingWindow = 100
	cfg.QueueTimeout = 10 * time.Second
	cfg.WindowGrowth = 500 // large growth to ensure match after one timeout

	q := NewMatchmakingQueue(cfg)
	tp := NewMockTimeProvider(time.Now())
	q.SetTimeProvider(tp)

	matched := make(chan []*Player, 1)
	q.OnMatchFound(func(players []*Player) {
		matched <- players
	})

	p1 := newTestPlayer(1000)
	p2 := newTestPlayer(1500)
	require.NoError(t, q.Enqueue(p1, ModeFFA, RegionAny))
	require.NoError(t, q.Enqueue(p2, ModeFFA, RegionAny))

	// No match yet.
	q.RunOnce()
	assert.Equal(t, 2, q.Size())

	// Advance clock past QueueTimeout.
	tp.Advance(cfg.QueueTimeout * 2)
	q.RunOnce()

	select {
	case players := <-matched:
		assert.Len(t, players, 2)
	case <-time.After(time.Second):
		t.Fatal("expected match after window growth")
	}
}

func TestRunOnce_DifferentModes_NoMatch(t *testing.T) {
	cfg := DefaultQueueConfig()
	cfg.MatchSize = 2

	q := NewMatchmakingQueue(cfg)
	matched := false
	q.OnMatchFound(func(_ []*Player) { matched = true })

	p1 := newTestPlayer(1500)
	p2 := newTestPlayer(1500)
	p2.PublicKey[0] = 99
	require.NoError(t, q.Enqueue(p1, ModeFFA, RegionAny))
	require.NoError(t, q.Enqueue(p2, ModeTeamDeathmatch, RegionAny))

	q.RunOnce()
	assert.False(t, matched, "players queued for different modes should not match")
}

func TestRunOnce_ThreePlayers_MatchSizeTwo(t *testing.T) {
	cfg := DefaultQueueConfig()
	cfg.MatchSize = 2
	cfg.RatingWindow = 1000

	q := NewMatchmakingQueue(cfg)
	matchCount := 0
	q.OnMatchFound(func(_ []*Player) { matchCount++ })

	for i := 0; i < 3; i++ {
		p := newTestPlayer(float64(1500 + i*10))
		p.PublicKey[0] = byte(i + 1)
		require.NoError(t, q.Enqueue(p, ModeFFA, RegionAny))
	}

	q.RunOnce()
	assert.Equal(t, 1, matchCount, "should form one match of 2 from 3 players")
	assert.Equal(t, 1, q.Size(), "one player should remain")
}

func TestSetTimeProvider(t *testing.T) {
	q := NewMatchmakingQueue(nil)
	tp := NewMockTimeProvider(time.Now())
	q.SetTimeProvider(tp)
	// Just ensure it doesn't panic.
	assert.Equal(t, 0, q.Size())
}
