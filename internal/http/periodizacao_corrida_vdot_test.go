package http

import (
	"testing"

	"staff_app/internal/daniels"
)

func TestRaceDistanceKM(t *testing.T) {
	cases := []struct {
		in     string
		wantKM float64
		wantOK bool
	}{
		{"5K", 5.0, true},
		{"10K", 10.0, true},
		{"21K", 21.1, true},
		{"42K", 42.2, true},
		{"3K", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := raceDistanceKM(tc.in)
		if ok != tc.wantOK || got != tc.wantKM {
			t.Fatalf("raceDistanceKM(%q)=(%v,%v) want (%v,%v)", tc.in, got, ok, tc.wantKM, tc.wantOK)
		}
	}
}

func TestVDOTUsesRaceDistanceNotHardcoded5K(t *testing.T) {
	paceSec := 300 // 05:00 /km

	dist5, _ := raceDistanceKM("5K")
	dist42, _ := raceDistanceKM("42K")

	vdot5, err := daniels.EstimateVDOTByRace(int(float64(paceSec)*dist5+0.5), dist5)
	if err != nil {
		t.Fatalf("vdot5: %v", err)
	}
	vdot42, err := daniels.EstimateVDOTByRace(int(float64(paceSec)*dist42+0.5), dist42)
	if err != nil {
		t.Fatalf("vdot42: %v", err)
	}

	// Old bug: always used paceSec*5 @ 5.0km — marathon incorrectly equaled 5K VDOT.
	buggy, err := daniels.EstimateVDOTByRace(paceSec*5, 5.0)
	if err != nil {
		t.Fatalf("buggy: %v", err)
	}
	if buggy != vdot5 {
		t.Fatalf("sanity: buggy 5K path should match correct 5K, got buggy=%.2f vdot5=%.2f", buggy, vdot5)
	}
	if vdot42 == buggy {
		t.Fatalf("marathon VDOT must use 42.2km race time, not hardcoded 5K; got 42K=%.2f buggy5K=%.2f", vdot42, buggy)
	}
	if vdot42 == vdot5 {
		t.Fatalf("expected different VDOT for 42K vs 5K at same pace; both=%.2f", vdot5)
	}
}
