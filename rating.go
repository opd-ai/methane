package methane

import "math"

// Glicko-2 algorithm constants.
const (
	// tau is the system volatility constraint (τ).
	tau = 0.5
	// convergenceEpsilon is the convergence tolerance for the volatility iteration.
	convergenceEpsilon = 0.000001
	// glicko2Scale is the conversion factor between Glicko-1 and Glicko-2 scales.
	glicko2Scale = 173.7178
	// defaultRatingValue is the default Glicko-2 rating.
	defaultRatingValue = 1500.0
	// defaultRD is the default rating deviation.
	defaultRD = 350.0
	// defaultSigma is the default volatility.
	defaultSigma = 0.06
)

// Rating holds Glicko-2 rating parameters for a player.
type Rating struct {
	// R is the rating (Glicko-2 scale, approximately 1500 for a new player).
	R float64
	// RD is the rating deviation.
	RD float64
	// Sigma is the volatility.
	Sigma float64
}

// DefaultRating returns the default Glicko-2 rating for a new player.
func DefaultRating() Rating {
	return Rating{
		R:     defaultRatingValue,
		RD:    defaultRD,
		Sigma: defaultSigma,
	}
}

// gFunc computes the g(φ) function used in Glicko-2.
func gFunc(phi float64) float64 {
	return 1.0 / math.Sqrt(1.0+3.0*phi*phi/(math.Pi*math.Pi))
}

// eFunc computes the E(µ, µⱼ, φⱼ) expected score function used in Glicko-2.
func eFunc(mu, muJ, phiJ float64) float64 {
	return 1.0 / (1.0 + math.Exp(-gFunc(phiJ)*(mu-muJ)))
}

// CalculateNewRating computes a new Glicko-2 rating after a rating period.
//
// player is the current rating of the player being updated.
// opponents is the slice of opponent ratings encountered during the period.
// scores is the corresponding outcomes (1.0=win, 0.5=draw, 0.0=loss).
//
// If the player did not compete (len(opponents)==0), only the deviation is
// inflated as specified by the Glicko-2 algorithm.
func CalculateNewRating(player Rating, opponents []Rating, scores []float64) Rating {
	// Convert to Glicko-2 internal scale.
	mu := (player.R - defaultRatingValue) / glicko2Scale
	phi := player.RD / glicko2Scale

	if len(opponents) == 0 {
		// Step 6: no games played – inflate deviation only.
		phiStar := math.Sqrt(phi*phi + player.Sigma*player.Sigma)
		return Rating{
			R:     player.R,
			RD:    phiStar * glicko2Scale,
			Sigma: player.Sigma,
		}
	}

	// Step 3: compute v (estimated variance).
	v := computeVariance(mu, phi, opponents)

	// Step 4: compute delta (estimated improvement).
	delta := computeDelta(mu, phi, opponents, scores, v)

	// Step 5: compute new volatility sigma'.
	sigma := computeNewVolatility(phi, player.Sigma, delta, v)

	// Step 6: update phi to phi*.
	phiStar := math.Sqrt(phi*phi + sigma*sigma)

	// Step 7: compute new phi' and mu'.
	phiPrime := 1.0 / math.Sqrt(1.0/phiStar/phiStar+1.0/v)
	muPrime := mu + phiPrime*phiPrime*sumGTimesScore(mu, opponents, scores)

	// Convert back to Glicko-1 scale.
	return Rating{
		R:     glicko2Scale*muPrime + defaultRatingValue,
		RD:    glicko2Scale * phiPrime,
		Sigma: sigma,
	}
}

// computeVariance computes the estimated variance v (step 3).
func computeVariance(mu, _ float64, opponents []Rating) float64 {
	var sum float64
	for _, opp := range opponents {
		muJ := (opp.R - defaultRatingValue) / glicko2Scale
		phiJ := opp.RD / glicko2Scale
		gJ := gFunc(phiJ)
		eJ := eFunc(mu, muJ, phiJ)
		sum += gJ * gJ * eJ * (1.0 - eJ)
	}
	if sum == 0 {
		return math.MaxFloat64
	}
	return 1.0 / sum
}

// computeDelta computes the estimated improvement delta (step 4).
func computeDelta(mu, _ float64, opponents []Rating, scores []float64, v float64) float64 {
	return v * sumGTimesScore(mu, opponents, scores)
}

// sumGTimesScore computes sum(g(φⱼ) * (sⱼ - E)) for all opponents.
func sumGTimesScore(mu float64, opponents []Rating, scores []float64) float64 {
	var sum float64
	for i, opp := range opponents {
		muJ := (opp.R - defaultRatingValue) / glicko2Scale
		phiJ := opp.RD / glicko2Scale
		gJ := gFunc(phiJ)
		eJ := eFunc(mu, muJ, phiJ)
		sum += gJ * (scores[i] - eJ)
	}
	return sum
}

// computeNewVolatility implements the iterative Illinois algorithm (step 5).
func computeNewVolatility(phi, sigma, delta, v float64) float64 {
	a := math.Log(sigma * sigma)
	f := func(x float64) float64 {
		ex := math.Exp(x)
		d := phi*phi + v + ex
		return ex*(delta*delta-phi*phi-v-ex)/(2.0*d*d) - (x-a)/(tau*tau)
	}

	// Initial bracket.
	A := a
	var B float64
	if delta*delta > phi*phi+v {
		B = math.Log(delta*delta - phi*phi - v)
	} else {
		k := 1.0
		for f(a-k*tau) < 0 {
			k++
		}
		B = a - k*tau
	}

	fA := f(A)
	fB := f(B)

	// Illinois algorithm iteration.
	for math.Abs(B-A) > convergenceEpsilon {
		C := A + (A-B)*fA/(fB-fA)
		fC := f(C)
		if fC*fB <= 0 {
			A = B
			fA = fB
		} else {
			fA /= 2.0
		}
		B = C
		fB = fC
	}
	return math.Exp(A / 2.0)
}
