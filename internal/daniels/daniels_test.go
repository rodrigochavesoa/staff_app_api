package daniels

import (
	"math"
	"testing"
)

func TestCalculate3kTest(t *testing.T) {
	tests := []struct {
		name           string
		timeSeconds    int
		expectedVDOT   float64
		expectedFTP    int
		expectedPaceZ2 int // base aerobic pace (max)
		wantErr        bool
	}{
		{
			name:           "3km in 12min (720s) - Golden Case",
			timeSeconds:    720,
			expectedVDOT:   47.9,
			expectedFTP:    254,
			expectedPaceZ2: 326,
			wantErr:        false,
		},
		{
			name:           "3km in 15min (900s) - Golden Case",
			timeSeconds:    900,
			expectedVDOT:   37.0,
			expectedFTP:    318,
			expectedPaceZ2: 407,
			wantErr:        false,
		},
		{
			name:           "3km in 10min (600s) - Golden Case",
			timeSeconds:    600,
			expectedVDOT:   58.8,
			expectedFTP:    212,
			expectedPaceZ2: 271,
			wantErr:        false,
		},
		{
			name:           "3km in 14:30 (870s) - Golden Case",
			timeSeconds:    870,
			expectedVDOT:   38.5,
			expectedFTP:    307,
			expectedPaceZ2: 393, // 307.4 * 1.28 = 393.472 -> 393
			wantErr:        false,
		},
		{
			name:           "3km in 20min (1200s) - Golden Case",
			timeSeconds:    1200,
			expectedVDOT:   26.3,
			expectedFTP:    424,
			expectedPaceZ2: 543, // 424 * 1.28 = 542.72 -> 543
			wantErr:        false,
		},
		{
			name:           "Limit case: Fast boundary (300s)",
			timeSeconds:    300,
			expectedVDOT:   130.3, // Raw formula result (no clamp)
			expectedFTP:    106,
			expectedPaceZ2: 136,
			wantErr:        false,
		},
		{
			name:           "Limit case: Slow boundary (3600s)",
			timeSeconds:    3600,
			expectedVDOT:   5.4, // Raw formula result (no clamp)
			expectedFTP:    1272,
			expectedPaceZ2: 1628,
			wantErr:        false,
		},
		{
			name:        "Outside limit: too fast (299s)",
			timeSeconds: 299,
			wantErr:     true,
		},
		{
			name:        "Outside limit: too slow (3601s)",
			timeSeconds: 3601,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Calculate3kTest(tt.timeSeconds)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Calculate3kTest() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Verify VDOT
			if math.Abs(res.VDOT-tt.expectedVDOT) > 0.05 {
				t.Errorf("VDOT mismatch: got %.2f, expected %.2f", res.VDOT, tt.expectedVDOT)
			}

			// Verify FTP Pace
			if res.FTPPaceSeconds != tt.expectedFTP {
				t.Errorf("FTP Pace mismatch: got %d, expected %d", res.FTPPaceSeconds, tt.expectedFTP)
			}

			// Verify Z2 Max Pace
			if res.PaceZ2Max != tt.expectedPaceZ2 {
				t.Errorf("Pace Z2 Max mismatch: got %d, expected %d", res.PaceZ2Max, tt.expectedPaceZ2)
			}
		})
	}
}

func TestEstimateVDOTByRace(t *testing.T) {
	tests := []struct {
		name         string
		timeSeconds  int
		distanceKM   float64
		expectedVDOT float64
		wantErr      bool
	}{
		{
			name:         "5K in 20min (1200s)",
			timeSeconds:  1200,
			distanceKM:   5.0,
			expectedVDOT: 49.96, // ~50.0
			wantErr:      false,
		},
		{
			name:         "10K in 45min (2700s)",
			timeSeconds:  2700,
			distanceKM:   10.0,
			expectedVDOT: 46.62, // ~46.6
			wantErr:      false,
		},
		{
			name:         "Half Marathon in 1h45 (6300s)",
			timeSeconds:  6300,
			distanceKM:   21.1,
			expectedVDOT: 43.64, // ~43.6
			wantErr:      false,
		},
		{
			name:        "Invalid zero distance",
			timeSeconds: 1200,
			distanceKM:  0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vdot, err := EstimateVDOTByRace(tt.timeSeconds, tt.distanceKM)
			if (err != nil) != tt.wantErr {
				t.Fatalf("EstimateVDOTByRace() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if math.Abs(vdot-tt.expectedVDOT) > 0.1 {
				t.Errorf("VDOT estimate mismatch: got %.2f, expected %.2f", vdot, tt.expectedVDOT)
			}
		})
	}
}

func TestCalculateZonesInterpolation(t *testing.T) {
	// Table VDOT points test:
	// VDOT 40 -> E should be 307
	// VDOT 42.5 -> E should be interpolated between VDOT 40 (307) and VDOT 45 (287)
	// (307 + 287) / 2 = 297
	tests := []struct {
		vdot         float64
		zone         string
		expectedPace int
	}{
		{vdot: 30.0, zone: "E", expectedPace: 362},
		{vdot: 40.0, zone: "E", expectedPace: 307},
		{vdot: 45.0, zone: "E", expectedPace: 287},
		{vdot: 42.5, zone: "E", expectedPace: 297}, // midpoint of 40 and 45
		{vdot: 50.0, zone: "M", expectedPace: 252},
		{vdot: 52.5, zone: "M", expectedPace: 245}, // midpoint of 50 (252) and 55 (238) = 245
		{vdot: 25.0, zone: "T", expectedPace: 310}, // clamped to 30.0 value
		{vdot: 90.0, zone: "R", expectedPace: 155}, // clamped to 70.0 value
	}

	for _, tt := range tests {
		zones := CalculateZones(tt.vdot)
		z, exists := zones[tt.zone]
		if !exists {
			t.Fatalf("Zone %s not returned in CalculateZones", tt.zone)
		}
		if z.PaceAlvo != tt.expectedPace {
			t.Errorf("VDOT %.1f Zone %s pace mismatch: got %d, expected %d", tt.vdot, tt.zone, z.PaceAlvo, tt.expectedPace)
		}
	}
}
