package daniels

import (
	"errors"
	"math"
)

// TestResult represents the VDOT and corresponding FTP zones calculated from a 3km run test.
type TestResult struct {
	VDOT           float64 `json:"vdot"`
	FTPPaceSeconds int     `json:"ftp_pace_seconds"`
	PaceZ1Min      int     `json:"pace_z1_min"`
	PaceZ1Max      int     `json:"pace_z1_max"`
	PaceZ2Min      int     `json:"pace_z2_min"`
	PaceZ2Max      int     `json:"pace_z2_max"`
	PaceZ3Min      int     `json:"pace_z3_min"`
	PaceZ3Max      int     `json:"pace_z3_max"`
	PaceZ4Min      int     `json:"pace_z4_min"`
	PaceZ4Max      int     `json:"pace_z4_max"`
	PaceZ5Min      int     `json:"pace_z5_min"`
	PaceZ5Max      int     `json:"pace_z5_max"`
}

// Calculate3kTest computes VDOT and zones from a 3 km test time in seconds.
// Note: Time range validation (300s to 3600s) has been moved here from the legacy Flask route (vdot_salvar) for robustness.
func Calculate3kTest(timeSeconds int) (TestResult, error) {
	if timeSeconds < 300 || timeSeconds > 3600 {
		return TestResult{}, errors.New("time out of plausible range for 3 km test (5 min - 60 min)")
	}

	t := float64(timeSeconds) / 60.0 // time in minutes
	v := 3000.0 / t                  // speed in m/min

	// Jack Daniels Formula (VO2max equivalent)
	pctVO2 := 0.8 + 0.1894393*math.Exp(-0.012778*t) + 0.2989558*math.Exp(-0.1932605*t)
	vo2 := -4.60 + 0.182258*v + 0.000104*math.Pow(v, 2.0)
	vdot := math.Round((vo2/pctVO2)*10) / 10 // Round to 1 decimal place

	// Pace in seconds/km
	pace3km := float64(timeSeconds) / 3.0 // seconds/km
	ftp := pace3km * 1.06                 // FTP is +6% of 3km test pace

	ftpRound := math.Round(ftp)

	// Zones based on FTP Pace (larger = slower)
	result := TestResult{
		VDOT:           vdot,
		FTPPaceSeconds: int(ftpRound),
		PaceZ5Min:      int(math.Round(ftp * 0.75)),
		PaceZ5Max:      int(math.Round(ftp * 0.87)),
		PaceZ4Min:      int(math.Round(ftp * 0.88)),
		PaceZ4Max:      int(math.Round(ftp * 0.99)),
		PaceZ3Min:      int(math.Round(ftp * 1.00)),
		PaceZ3Max:      int(math.Round(ftp * 1.13)),
		PaceZ2Min:      int(math.Round(ftp * 1.14)),
		PaceZ2Max:      int(math.Round(ftp * 1.28)),
		PaceZ1Min:      int(math.Round(ftp * 1.29)),
		PaceZ1Max:      int(math.Round(ftp * 1.43)),
	}

	return result, nil
}

// EstimateVDOTByRace calculates VDOT from any race time and distance.
func EstimateVDOTByRace(timeSeconds int, distanceKM float64) (float64, error) {
	if timeSeconds <= 0 || distanceKM <= 0 {
		return 0, errors.New("time and distance must be positive numbers")
	}

	timeMin := float64(timeSeconds) / 60.0
	speedMMin := (distanceKM * 1000.0) / timeMin

	// Jack Daniels Formula for VDOT: VO2 = -4.60 + 0.182258 * v + 0.000104 * v^2
	vo2 := -4.60 + (0.182258 * speedMMin) + (0.000104 * math.Pow(speedMMin, 2.0))

	// Estimate percent of VO2max based on duration
	var percentual float64
	if timeMin <= 8 {
		percentual = 1.00
	} else if timeMin <= 15 {
		percentual = 0.98
	} else if timeMin <= 30 {
		percentual = 0.95
	} else if timeMin <= 90 {
		percentual = 0.88
	} else {
		percentual = 0.83
	}

	vdot := vo2 / percentual

	// Clamp between 20 and 85
	if vdot < 20.0 {
		vdot = 20.0
	} else if vdot > 85.0 {
		vdot = 85.0
	}

	return vdot, nil
}
