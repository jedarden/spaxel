// Package tracking provides biomechanical blob tracking using an Unscented Kalman Filter.
package tracking

import (
	"math"
)

// ukfState is [x, z, vx, vz] — floor plane position and velocity.
// (y=height is not tracked; blobs are projected onto floor)
const stateN = 4

// ukfSigmaPoints returns 2n+1 sigma points for a state vector x and covariance P.
// alpha=1e-3, beta=2, kappa=0 are standard choices for UKF.
func ukfSigmaPoints(x [stateN]float64, P [stateN][stateN]float64) [][stateN]float64 {
	n := float64(stateN)
	alpha := 1e-3
	kappa := 0.0
	lambda := alpha*alpha*(n+kappa) - n
	scale := math.Sqrt(n + lambda)

	// Compute lower Cholesky factor of P.
	L := choleskyN(P)

	sigma := make([][stateN]float64, 2*stateN+1)
	sigma[0] = x
	for i := 0; i < stateN; i++ {
		// x + scale * L[:,i]
		for j := 0; j < stateN; j++ {
			sigma[1+i][j] = x[j] + scale*L[j][i]
			sigma[1+stateN+i][j] = x[j] - scale*L[j][i]
		}
	}
	return sigma
}

// ukfWeights returns mean and covariance weights for sigma points.
func ukfWeights() (wm, wc [2*stateN + 1]float64) {
	n := float64(stateN)
	alpha := 1e-3
	beta := 2.0
	kappa := 0.0
	lambda := alpha*alpha*(n+kappa) - n

	wm[0] = lambda / (n + lambda)
	wc[0] = wm[0] + (1 - alpha*alpha + beta)
	w := 1.0 / (2 * (n + lambda))
	for i := 1; i <= 2*stateN; i++ {
		wm[i] = w
		wc[i] = w
	}
	return
}

// processModel advances a state one time step dt under constant-velocity motion.
func processModel(x [stateN]float64, dt float64) [stateN]float64 {
	return [stateN]float64{
		x[0] + x[2]*dt, // x' = x + vx*dt
		x[1] + x[3]*dt, // z' = z + vz*dt
		x[2],           // vx unchanged
		x[3],           // vz unchanged
	}
}

// measureModel extracts measurement [x, z] from state.
func measureModel(x [stateN]float64) [2]float64 {
	return [2]float64{x[0], x[1]}
}

// UKF is an Unscented Kalman Filter tracking [x, z, vx, vz].
type UKF struct {
	X  [stateN]float64         // state estimate
	P  [stateN][stateN]float64 // covariance estimate
	Q  [stateN][stateN]float64 // process noise
	R  [2][2]float64           // measurement noise
}

// NewUKF creates a UKF at initial position (x0, z0).
func NewUKF(x0, z0 float64) *UKF {
	u := &UKF{}
	u.X = [stateN]float64{x0, z0, 0, 0}

	// Initial covariance: moderate position uncertainty, high velocity uncertainty.
	u.P[0][0] = 0.25
	u.P[1][1] = 0.25
	u.P[2][2] = 1.0
	u.P[3][3] = 1.0

	// Process noise: human walking (σ_pos ≈ 0.05m, σ_vel ≈ 0.5m/s per step)
	u.Q[0][0] = 0.0025
	u.Q[1][1] = 0.0025
	u.Q[2][2] = 0.25
	u.Q[3][3] = 0.25

	// Measurement noise: localization accuracy ≈ 0.4m std dev
	u.R[0][0] = 0.16
	u.R[1][1] = 0.16

	return u
}

// Predict performs the UKF time-update for time step dt.
func (u *UKF) Predict(dt float64) {
	sigma := ukfSigmaPoints(u.X, u.P)
	wm, wc := ukfWeights()

	// Propagate sigma points.
	propSigma := make([][stateN]float64, len(sigma))
	for i, sp := range sigma {
		propSigma[i] = processModel(sp, dt)
	}

	// Compute predicted mean.
	var xPred [stateN]float64
	for i, sp := range propSigma {
		for j := 0; j < stateN; j++ {
			xPred[j] += wm[i] * sp[j]
		}
	}

	// Compute predicted covariance.
	var pPred [stateN][stateN]float64
	for i, sp := range propSigma {
		var diff [stateN]float64
		for j := 0; j < stateN; j++ {
			diff[j] = sp[j] - xPred[j]
		}
		for r := 0; r < stateN; r++ {
			for c := 0; c < stateN; c++ {
				pPred[r][c] += wc[i] * diff[r] * diff[c]
			}
		}
	}
	// Add process noise.
	for r := 0; r < stateN; r++ {
		for c := 0; c < stateN; c++ {
			pPred[r][c] += u.Q[r][c]
		}
	}

	u.X = xPred
	u.P = pPred

	// Biomechanical constraint: cap velocity to human walking speed.
	const maxVel = 3.0
	if u.X[2] > maxVel {
		u.X[2] = maxVel
	} else if u.X[2] < -maxVel {
		u.X[2] = -maxVel
	}
	if u.X[3] > maxVel {
		u.X[3] = maxVel
	} else if u.X[3] < -maxVel {
		u.X[3] = -maxVel
	}
}

// Update performs the UKF measurement-update given measurement z = [x, z].
func (u *UKF) Update(meas [2]float64) {
	sigma := ukfSigmaPoints(u.X, u.P)
	_, wc := ukfWeights()
	wm, _ := ukfWeights()

	// Predicted measurement and cross-covariance.
	zSigma := make([][2]float64, len(sigma))
	var zMean [2]float64
	for i, sp := range sigma {
		zSigma[i] = measureModel(sp)
		for j := 0; j < 2; j++ {
			zMean[j] += wm[i] * zSigma[i][j]
		}
	}

	// Innovation covariance Szz + R.
	var Szz [2][2]float64
	var Sxz [stateN][2]float64
	for i, sp := range sigma {
		zDiff := [2]float64{zSigma[i][0] - zMean[0], zSigma[i][1] - zMean[1]}
		xDiff := [stateN]float64{}
		for j := 0; j < stateN; j++ {
			xDiff[j] = sp[j] - u.X[j]
		}
		for r := 0; r < 2; r++ {
			for c := 0; c < 2; c++ {
				Szz[r][c] += wc[i] * zDiff[r] * zDiff[c]
			}
		}
		for r := 0; r < stateN; r++ {
			for c := 0; c < 2; c++ {
				Sxz[r][c] += wc[i] * xDiff[r] * zDiff[c]
			}
		}
	}
	// Add measurement noise.
	Szz[0][0] += u.R[0][0]
	Szz[1][1] += u.R[1][1]

	// Kalman gain K = Sxz * Szz^-1.
	det := Szz[0][0]*Szz[1][1] - Szz[0][1]*Szz[1][0]
	if math.Abs(det) < 1e-10 {
		return
	}
	invSzz := [2][2]float64{
		{Szz[1][1] / det, -Szz[0][1] / det},
		{-Szz[1][0] / det, Szz[0][0] / det},
	}

	// K = Sxz * invSzz  (stateN×2)
	var K [stateN][2]float64
	for r := 0; r < stateN; r++ {
		for c := 0; c < 2; c++ {
			for k := 0; k < 2; k++ {
				K[r][c] += Sxz[r][k] * invSzz[k][c]
			}
		}
	}

	// Innovation.
	innov := [2]float64{meas[0] - zMean[0], meas[1] - zMean[1]}

	// Update state and covariance.
	for j := 0; j < stateN; j++ {
		u.X[j] += K[j][0]*innov[0] + K[j][1]*innov[1]
	}
	// P -= K * Szz * Kᵀ
	for r := 0; r < stateN; r++ {
		for c := 0; c < stateN; c++ {
			for k := 0; k < 2; k++ {
				for l := 0; l < 2; l++ {
					u.P[r][c] -= K[r][k] * Szz[k][l] * K[c][l]
				}
			}
		}
	}
}

// Position returns (x, z) estimated position.
func (u *UKF) Position() (float64, float64) {
	return u.X[0], u.X[1]
}

// Velocity returns (vx, vz) estimated velocity.
func (u *UKF) Velocity() (float64, float64) {
	return u.X[2], u.X[3]
}

// ─── Linear Algebra Helpers ─────────────────────────────────────────────────

// choleskyN computes the lower-triangular Cholesky factorisation of a stateN×stateN PSD matrix.
// Fallback: if matrix is not PD, returns identity scaled by small value.
func choleskyN(A [stateN][stateN]float64) [stateN][stateN]float64 {
	var L [stateN][stateN]float64
	for i := 0; i < stateN; i++ {
		for j := 0; j <= i; j++ {
			sum := A[i][j]
			for k := 0; k < j; k++ {
				sum -= L[i][k] * L[j][k]
			}
			if i == j {
				if sum <= 0 {
					sum = 1e-6
				}
				L[i][j] = math.Sqrt(sum)
			} else {
				if L[j][j] == 0 {
					L[i][j] = 0
				} else {
					L[i][j] = sum / L[j][j]
				}
			}
		}
	}
	return L
}
