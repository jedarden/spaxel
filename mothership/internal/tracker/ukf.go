// Package tracker provides biomechanical blob tracking using a full 3-D
// Unscented Kalman Filter with human-motion constraints.
package tracker

import (
	"math"

	"gonum.org/v1/gonum/mat"
)

// State vector: [x, y, z, vx, vy, vz]
// Coordinate system matches fusion.Blob: X and Z are the floor-plane axes,
// Y is height above the floor.
const (
	stateN = 6
	measN  = 3
)

// UKF scaling parameters — α=1 keeps weights well-conditioned for n=6.
const (
	ukfAlpha = 1.0
	ukfBeta  = 2.0
	ukfKappa = 0.0
)

// Biomechanical constraints for human motion.
const (
	maxHorizVel = 2.0 // m/s   horizontal speed cap
	maxVertVel  = 0.8 // m/s   vertical speed cap (gravity-consistent)
	maxAccelHz  = 3.0 // m/s²  horizontal acceleration cap
	minTurnRad  = 0.3 // m     minimum turning radius
)

// UKF is a 6-state Unscented Kalman Filter tracking a single entity in 3-D.
type UKF struct {
	X *mat.VecDense // state [x, y, z, vx, vy, vz]
	P *mat.Dense    // 6×6 state covariance
	Q *mat.Dense    // 6×6 process noise
	R *mat.Dense    // 3×3 measurement noise
}

// NewUKF creates a UKF seeded at world position (x0, y0, z0) with zero velocity.
func NewUKF(x0, y0, z0 float64) *UKF {
	u := &UKF{
		X: mat.NewVecDense(stateN, []float64{x0, y0, z0, 0, 0, 0}),
		P: mat.NewDense(stateN, stateN, nil),
		Q: mat.NewDense(stateN, stateN, nil),
		R: mat.NewDense(measN, measN, nil),
	}

	// Initial covariance — moderate position, lower height, high velocity uncertainty.
	u.P.Set(0, 0, 0.25)
	u.P.Set(1, 1, 0.09)
	u.P.Set(2, 2, 0.25)
	u.P.Set(3, 3, 1.0)
	u.P.Set(4, 4, 0.09)
	u.P.Set(5, 5, 1.0)

	// Process noise: human walking dynamics.
	u.Q.Set(0, 0, 2.5e-3)
	u.Q.Set(1, 1, 1.0e-3)
	u.Q.Set(2, 2, 2.5e-3)
	u.Q.Set(3, 3, 0.25)
	u.Q.Set(4, 4, 0.04)
	u.Q.Set(5, 5, 0.25)

	// Measurement noise: fusion localisation ≈0.4 m std-dev.
	u.R.Set(0, 0, 0.16)
	u.R.Set(1, 1, 0.16)
	u.R.Set(2, 2, 0.16)

	return u
}

// Predict performs the UKF time-update for a step of dt seconds.
func (u *UKF) Predict(dt float64) {
	prevVx, prevVy, prevVz := u.X.AtVec(3), u.X.AtVec(4), u.X.AtVec(5)

	sigma, wm, wc := u.sigmaPoints()

	// Propagate sigma points through constant-velocity model.
	prop := make([][]float64, len(sigma))
	for i, sp := range sigma {
		prop[i] = []float64{
			sp[0] + sp[3]*dt,
			sp[1] + sp[4]*dt,
			sp[2] + sp[5]*dt,
			sp[3], sp[4], sp[5],
		}
	}

	// Predicted mean.
	xp := make([]float64, stateN)
	for i, sp := range prop {
		w := wm[i]
		for j := range sp {
			xp[j] += w * sp[j]
		}
	}

	// Predicted covariance.
	pPred := mat.NewDense(stateN, stateN, nil)
	dv := mat.NewVecDense(stateN, nil)
	ov := mat.NewDense(stateN, stateN, nil)
	for i, sp := range prop {
		for j := 0; j < stateN; j++ {
			dv.SetVec(j, sp[j]-xp[j])
		}
		ov.Outer(wc[i], dv, dv)
		pPred.Add(pPred, ov)
	}
	pPred.Add(pPred, u.Q)

	u.X = mat.NewVecDense(stateN, xp)
	u.P = pPred

	u.applyConstraints(dt, prevVx, prevVy, prevVz)
}

// Update performs the UKF measurement-update given observation meas=[x,y,z].
func (u *UKF) Update(meas [measN]float64) {
	sigma, wm, wc := u.sigmaPoints()

	// Predicted measurement mean (first 3 components of state).
	zp := make([]float64, measN)
	for i, sp := range sigma {
		for j := 0; j < measN; j++ {
			zp[j] += wm[i] * sp[j]
		}
	}

	// Innovation covariance Szz and cross-covariance Sxz.
	Szz := mat.NewDense(measN, measN, nil)
	Sxz := mat.NewDense(stateN, measN, nil)
	zd := mat.NewVecDense(measN, nil)
	xd := mat.NewVecDense(stateN, nil)
	ozz := mat.NewDense(measN, measN, nil)
	oxz := mat.NewDense(stateN, measN, nil)
	for i, sp := range sigma {
		for j := 0; j < measN; j++ {
			zd.SetVec(j, sp[j]-zp[j])
		}
		for j := 0; j < stateN; j++ {
			xd.SetVec(j, sp[j]-u.X.AtVec(j))
		}
		ozz.Outer(wc[i], zd, zd)
		Szz.Add(Szz, ozz)
		oxz.Outer(wc[i], xd, zd)
		Sxz.Add(Sxz, oxz)
	}
	Szz.Add(Szz, u.R)

	// Kalman gain K = Sxz * Szz⁻¹.
	SzzInv := mat.NewDense(measN, measN, nil)
	if err := SzzInv.Inverse(Szz); err != nil {
		return // numerically singular — skip update
	}
	K := mat.NewDense(stateN, measN, nil)
	K.Mul(Sxz, SzzInv)

	// State update: X += K * (meas − zp).
	innov := mat.NewVecDense(measN, []float64{
		meas[0] - zp[0], meas[1] - zp[1], meas[2] - zp[2],
	})
	delta := mat.NewVecDense(stateN, nil)
	delta.MulVec(K, innov)
	u.X.AddVec(u.X, delta)

	// Covariance update: P = P − K*Szz*Kᵀ.
	KSzz := mat.NewDense(stateN, measN, nil)
	KSzz.Mul(K, Szz)
	KSzzKt := mat.NewDense(stateN, stateN, nil)
	KSzzKt.Mul(KSzz, K.T())
	newP := mat.NewDense(stateN, stateN, nil)
	newP.Sub(u.P, KSzzKt)
	symmetrizePD(newP)
	u.P = newP
}

// Position returns the estimated (x, y, z) position in metres.
func (u *UKF) Position() (x, y, z float64) {
	return u.X.AtVec(0), u.X.AtVec(1), u.X.AtVec(2)
}

// Velocity returns the estimated (vx, vy, vz) velocity in m/s.
func (u *UKF) Velocity() (vx, vy, vz float64) {
	return u.X.AtVec(3), u.X.AtVec(4), u.X.AtVec(5)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// sigmaPoints generates 2n+1 sigma points with their mean and covariance weights.
func (u *UKF) sigmaPoints() (sigma [][]float64, wm, wc []float64) {
	n := float64(stateN)
	lambda := ukfAlpha*ukfAlpha*(n+ukfKappa) - n
	c := n + lambda // = 6 when alpha=1, kappa=0

	// mat.Cholesky requires a mat.Symmetric; build c*P as SymDense.
	scaledP := mat.NewSymDense(stateN, nil)
	for i := 0; i < stateN; i++ {
		for j := i; j < stateN; j++ {
			v := c * u.P.At(i, j)
			scaledP.SetSym(i, j, v)
		}
	}
	ensurePDSym(scaledP)

	var chol mat.Cholesky
	if !chol.Factorize(scaledP) {
		// Fallback to small scaled identity.
		scaledP = mat.NewSymDense(stateN, nil)
		for i := 0; i < stateN; i++ {
			scaledP.SetSym(i, i, c*1e-4)
		}
		chol.Factorize(scaledP)
	}
	L := mat.NewTriDense(stateN, mat.Lower, nil)
	chol.LTo(L)

	xd := u.X.RawVector().Data
	sigma = make([][]float64, 2*stateN+1)
	sigma[0] = make([]float64, stateN)
	copy(sigma[0], xd)
	for i := 0; i < stateN; i++ {
		plus := make([]float64, stateN)
		minus := make([]float64, stateN)
		for j := 0; j < stateN; j++ {
			plus[j] = xd[j] + L.At(j, i)
			minus[j] = xd[j] - L.At(j, i)
		}
		sigma[1+i] = plus
		sigma[1+stateN+i] = minus
	}

	wm0 := lambda / c
	wc0 := wm0 + (1 - ukfAlpha*ukfAlpha + ukfBeta)
	wi := 0.5 / c
	wm = make([]float64, 2*stateN+1)
	wc = make([]float64, 2*stateN+1)
	wm[0] = wm0
	wc[0] = wc0
	for i := 1; i <= 2*stateN; i++ {
		wm[i] = wi
		wc[i] = wi
	}
	return
}

// applyConstraints enforces biomechanical limits given the pre-predict velocity.
func (u *UKF) applyConstraints(dt, prevVx, prevVy, prevVz float64) {
	vx := u.X.AtVec(3)
	vy := u.X.AtVec(4)
	vz := u.X.AtVec(5)

	if dt > 1e-6 {
		// Horizontal acceleration cap.
		dvx := vx - prevVx
		dvz := vz - prevVz
		dv := math.Sqrt(dvx*dvx + dvz*dvz)
		if dv/dt > maxAccelHz {
			s := maxAccelHz * dt / dv
			vx = prevVx + dvx*s
			vz = prevVz + dvz*s
		}

		// Turning radius constraint (only when moving).
		horizSpd := math.Sqrt(vx*vx + vz*vz)
		prevHorizSpd := math.Sqrt(prevVx*prevVx + prevVz*prevVz)
		if horizSpd > 0.15 && prevHorizSpd > 0.15 {
			prevHead := math.Atan2(prevVz, prevVx)
			newHead := math.Atan2(vz, vx)
			dHead := angleWrap(newHead - prevHead)
			maxTurn := horizSpd * dt / minTurnRad
			if math.Abs(dHead) > maxTurn {
				limited := prevHead + math.Copysign(maxTurn, dHead)
				vx = horizSpd * math.Cos(limited)
				vz = horizSpd * math.Sin(limited)
			}
		}
	}

	// Horizontal speed cap.
	if hs := math.Sqrt(vx*vx + vz*vz); hs > maxHorizVel {
		s := maxHorizVel / hs
		vx *= s
		vz *= s
	}

	// Vertical speed cap (gravity-consistent — limits upward and downward).
	if vy > maxVertVel {
		vy = maxVertVel
	} else if vy < -maxVertVel {
		vy = -maxVertVel
	}

	u.X.SetVec(3, vx)
	u.X.SetVec(4, vy)
	u.X.SetVec(5, vz)
}

// ensurePDSym adds minimal diagonal jitter to a SymDense to prevent non-positive pivots.
func ensurePDSym(A *mat.SymDense) {
	n := A.SymmetricDim()
	const jitter = 1e-8
	for i := 0; i < n; i++ {
		if A.At(i, i) < jitter {
			A.SetSym(i, i, jitter)
		}
	}
}

// symmetrizePD enforces symmetry and positive diagonal after covariance update.
func symmetrizePD(A *mat.Dense) {
	n, _ := A.Dims()
	const minDiag = 1e-9
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			avg := (A.At(i, j) + A.At(j, i)) * 0.5
			A.Set(i, j, avg)
			A.Set(j, i, avg)
		}
		if A.At(i, i) < minDiag {
			A.Set(i, i, minDiag)
		}
	}
}

// angleWrap folds an angle into (−π, π].
func angleWrap(a float64) float64 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}
