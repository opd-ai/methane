package methane

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRating(t *testing.T) {
	r := DefaultRating()
	assert.Equal(t, 1500.0, r.R)
	assert.Equal(t, 350.0, r.RD)
	assert.Equal(t, 0.06, r.Sigma)
}

func TestCalculateNewRating_NoGames(t *testing.T) {
	player := DefaultRating()
	result := CalculateNewRating(player, nil, nil)
	// Rating should remain 1500.
	assert.Equal(t, 1500.0, result.R)
	// RD should increase (deviation inflated).
	assert.Greater(t, result.RD, player.RD)
	// Volatility unchanged.
	assert.Equal(t, player.Sigma, result.Sigma)
}

func TestCalculateNewRating_SingleWin(t *testing.T) {
	player := Rating{R: 1500, RD: 200, Sigma: 0.06}
	opponent := Rating{R: 1400, RD: 30, Sigma: 0.06}
	result := CalculateNewRating(player, []Rating{opponent}, []float64{1.0})
	// Winning against a lower-rated player should increase rating modestly.
	assert.Greater(t, result.R, player.R)
	// Deviation should decrease after playing.
	assert.Less(t, result.RD, player.RD)
}

func TestCalculateNewRating_SingleLoss(t *testing.T) {
	player := Rating{R: 1500, RD: 200, Sigma: 0.06}
	opponent := Rating{R: 1600, RD: 30, Sigma: 0.06}
	result := CalculateNewRating(player, []Rating{opponent}, []float64{0.0})
	// Losing to a higher-rated player should decrease rating modestly.
	assert.Less(t, result.R, player.R)
}

func TestCalculateNewRating_SingleDraw(t *testing.T) {
	player := Rating{R: 1500, RD: 200, Sigma: 0.06}
	opponent := Rating{R: 1500, RD: 200, Sigma: 0.06}
	result := CalculateNewRating(player, []Rating{opponent}, []float64{0.5})
	// Draw against an equal opponent – rating should be close to 1500.
	assert.InDelta(t, 1500.0, result.R, 10.0)
}

func TestCalculateNewRating_MultipleOpponents(t *testing.T) {
	player := Rating{R: 1500, RD: 200, Sigma: 0.06}
	opponents := []Rating{
		{R: 1400, RD: 30, Sigma: 0.06},
		{R: 1550, RD: 100, Sigma: 0.06},
		{R: 1700, RD: 300, Sigma: 0.06},
	}
	scores := []float64{1.0, 0.0, 0.5}
	result := CalculateNewRating(player, opponents, scores)
	require.NotNil(t, result)
	// Result must be a valid finite number.
	assert.False(t, math.IsNaN(result.R), "rating should not be NaN")
	assert.False(t, math.IsInf(result.R, 0), "rating should not be Inf")
	assert.False(t, math.IsNaN(result.RD), "RD should not be NaN")
	assert.False(t, math.IsInf(result.RD, 0), "RD should not be Inf")
	assert.False(t, math.IsNaN(result.Sigma), "Sigma should not be NaN")
	assert.False(t, math.IsInf(result.Sigma, 0), "Sigma should not be Inf")
	assert.Greater(t, result.RD, 0.0)
	assert.Greater(t, result.Sigma, 0.0)
}

// TestGlicko2Example tests against the worked example from Glicko-2 paper
// (http://www.glicko.net/glicko/glicko2.pdf).
func TestGlicko2Example(t *testing.T) {
	// Player: r=1500, RD=200, sigma=0.06.
	player := Rating{R: 1500, RD: 200, Sigma: 0.06}
	opponents := []Rating{
		{R: 1400, RD: 30, Sigma: 0.06},
		{R: 1550, RD: 100, Sigma: 0.06},
		{R: 1700, RD: 300, Sigma: 0.06},
	}
	scores := []float64{1.0, 0.0, 0.0}

	result := CalculateNewRating(player, opponents, scores)

	// The Glicko-2 paper gives approximate results; we allow small deltas.
	// Expected: r' ≈ 1464.06, RD' ≈ 151.52, sigma' ≈ 0.05999
	assert.InDelta(t, 1464.0, result.R, 5.0, "rating should be ~1464")
	assert.InDelta(t, 151.5, result.RD, 5.0, "RD should be ~151.5")
	assert.InDelta(t, 0.06, result.Sigma, 0.01, "sigma should be ~0.06")
}

func TestGFunc(t *testing.T) {
	// g(0) should be 1.
	assert.InDelta(t, 1.0, gFunc(0), 0.0001)
	// g(phi) should be between 0 and 1 for positive phi.
	assert.Greater(t, gFunc(1.0), 0.0)
	assert.Less(t, gFunc(1.0), 1.0)
}

func TestEFunc(t *testing.T) {
	// E(mu, mu, 0) = 0.5 (equal players, phi=0).
	e := eFunc(0, 0, 0)
	assert.InDelta(t, 0.5, e, 0.0001)
	// E should be in (0,1).
	e2 := eFunc(0.5, -0.5, 1.0)
	assert.Greater(t, e2, 0.0)
	assert.Less(t, e2, 1.0)
}

func TestRatingDeviationDecreasesWithPlay(t *testing.T) {
	player := DefaultRating()
	opponent := Rating{R: 1500, RD: 50, Sigma: 0.06}
	result := CalculateNewRating(player, []Rating{opponent}, []float64{1.0})
	assert.Less(t, result.RD, player.RD, "RD should decrease after playing")
}

func TestVolatilityPositive(t *testing.T) {
	player := Rating{R: 1800, RD: 100, Sigma: 0.06}
	opponent := Rating{R: 1200, RD: 50, Sigma: 0.06}
	result := CalculateNewRating(player, []Rating{opponent}, []float64{0.0})
	assert.Greater(t, result.Sigma, 0.0, "volatility must remain positive")
}
