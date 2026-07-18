package garmin

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"staff_app/internal/domain"

	fitlib "github.com/tormoder/fit"
)

// FitFixtureFiles returns absolute paths to committed synthetic FIT fixtures
// under testdata/fit. Paths are resolved from this source file so tests work
// regardless of the process working directory (local or CI).
func FitFixtureFiles() ([]string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("unable to resolve garmin package path")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "testdata", "fit")
	// Only synthetic fixtures are part of the public package; device exports stay local.
	matches, err := filepath.Glob(filepath.Join(dir, "synthetic_*.fit"))
	if err != nil {
		return nil, err
	}
	return matches, nil
}

func ParseFITFile(path string) (*domain.GarminActivity, error) {
	// #nosec G304 - path is validated and sanitized by the caller handler to prevent path traversal
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening fit file: %w", err)
	}
	defer file.Close()

	decoded, err := fitlib.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decoding fit file: %w", err)
	}

	activityFile, err := decoded.Activity()
	if err != nil {
		return nil, fmt.Errorf("reading fit activity: %w", err)
	}

	activity := &domain.GarminActivity{ActivityType: decoded.Type().String()}
	if len(activityFile.Sessions) > 0 {
		applySession(activity, activityFile.Sessions[0])
	}

	for _, record := range activityFile.Records {
		if r := fitRecord(record); r != nil {
			activity.Records = append(activity.Records, *r)
		}
	}

	activity.AnalyticsSummary = AnalyticsFromActivity(CSVActivity{Activity: *activity})
	return activity, nil
}

func applySession(activity *domain.GarminActivity, session *fitlib.SessionMsg) {
	activity.ActivityType = session.Sport.String()
	if validTime(session.StartTime) {
		t := session.StartTime
		activity.StartTime = &t
	}
	if v := finite(session.GetTotalElapsedTimeScaled()); v != nil {
		activity.DurationSeconds = v
	}
	if v := finite(session.GetTotalDistanceScaled()); v != nil {
		activity.DistanceMeters = v
	}
	if session.TotalCalories != 0xFFFF {
		v := int(session.TotalCalories)
		activity.Calories = &v
	}
	if session.AvgHeartRate != 0xFF {
		v := float64(session.AvgHeartRate)
		activity.AvgHeartRate = &v
	}
	if session.MaxHeartRate != 0xFF {
		v := float64(session.MaxHeartRate)
		activity.MaxHeartRate = &v
	}
	if session.AvgCadence != 0xFF {
		v := float64(session.AvgCadence)
		activity.AvgCadence = &v
	}
	if session.TotalAscent != 0xFFFF {
		v := float64(session.TotalAscent)
		activity.ElevationGainM = &v
	}
	if session.TotalDescent != 0xFFFF {
		v := float64(session.TotalDescent)
		activity.ElevationLossM = &v
	}
	if session.AvgPower != 0xFFFF {
		v := float64(session.AvgPower)
		activity.AvgPowerWatts = &v
	}
	if session.ThresholdPower != 0xFFFF {
		v := float64(session.ThresholdPower)
		activity.ThresholdPower = &v
	}
	if v := finite(session.GetTotalTrainingEffectScaled()); v != nil {
		activity.AerobicTE = v
	}
	if v := finite(session.GetTotalAnaerobicTrainingEffectScaled()); v != nil {
		activity.AnaerobicTE = v
	}
	if v := speedKMH(session.GetEnhancedAvgSpeedScaled(), session.GetAvgSpeedScaled()); v != nil {
		activity.AvgSpeedKMH = v
	}
	if v := speedKMH(session.GetEnhancedMaxSpeedScaled(), session.GetMaxSpeedScaled()); v != nil {
		activity.MaxSpeedKMH = v
	}
}

func fitRecord(record *fitlib.RecordMsg) *domain.ActivityRecord {
	out := &domain.ActivityRecord{}
	if validTime(record.Timestamp) {
		t := record.Timestamp
		out.Timestamp = &t
	}
	if !record.PositionLat.Invalid() {
		v := record.PositionLat.Degrees()
		out.Latitude = &v
	}
	if !record.PositionLong.Invalid() {
		v := record.PositionLong.Degrees()
		out.Longitude = &v
	}
	if v := finite(record.GetEnhancedAltitudeScaled()); v != nil {
		out.AltitudeM = v
	} else if v := finite(record.GetAltitudeScaled()); v != nil {
		out.AltitudeM = v
	}
	if record.HeartRate != 0xFF {
		v := int(record.HeartRate)
		out.HeartRate = &v
	}
	if record.Cadence != 0xFF {
		v := int(record.Cadence)
		out.Cadence = &v
	}
	if v := speedKMH(record.GetEnhancedSpeedScaled(), record.GetSpeedScaled()); v != nil {
		out.SpeedKMH = v
	}
	if record.Power != 0xFFFF {
		v := int(record.Power)
		power := float64(v)
		out.PowerWatts = &power
	}

	raw := map[string]any{
		"timestamp":  out.Timestamp,
		"latitude":   out.Latitude,
		"longitude":  out.Longitude,
		"altitude":   out.AltitudeM,
		"heart_rate": out.HeartRate,
		"cadence":    out.Cadence,
		"speed_kmh":  out.SpeedKMH,
		"power":      out.PowerWatts,
	}
	if b, err := json.Marshal(raw); err == nil {
		out.RawData = string(b)
	}

	if out.Timestamp == nil && out.Latitude == nil && out.Longitude == nil && out.HeartRate == nil {
		return nil
	}
	return out
}

func speedKMH(values ...float64) *float64 {
	for _, value := range values {
		if v := finite(value); v != nil {
			kmh := *v * 3.6
			return &kmh
		}
	}
	return nil
}

func finite(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

func validTime(t time.Time) bool {
	return !t.IsZero() && t.Year() > 1990
}
