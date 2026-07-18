package garmin

import (
	"path/filepath"
	"testing"
)

func TestParseFITFixtures(t *testing.T) {
	files, err := FitFixtureFiles()
	if err != nil {
		t.Fatalf("failed to resolve fit fixtures: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one FIT fixture in testdata/fit")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			activity, err := ParseFITFile(file)
			if err != nil {
				t.Fatalf("ParseFITFile returned error: %v", err)
			}
			if activity.ActivityType == "" {
				t.Errorf("expected activity type")
			}
			if activity.StartTime == nil {
				t.Errorf("expected start time")
			}
			if activity.DurationSeconds == nil || *activity.DurationSeconds <= 0 {
				t.Errorf("expected positive duration, got %v", activity.DurationSeconds)
			}
			if activity.DistanceMeters == nil || *activity.DistanceMeters <= 0 {
				t.Errorf("expected positive distance, got %v", activity.DistanceMeters)
			}
			if len(activity.Records) == 0 {
				t.Errorf("expected detailed records")
			}
			if activity.AnalyticsSummary == nil {
				t.Errorf("expected analytics summary")
			}
		})
	}
}
