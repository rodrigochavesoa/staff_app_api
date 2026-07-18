package garmin

import (
	"strings"
	"testing"
)

func TestParseCSVGarminExport(t *testing.T) {
	input := strings.NewReader(`Tipo de atividade;Data;Título;Distância;Calorias;Tempo;FC Média;FC máxima;Cadência de corrida média;Ritmo médio;Melhor ritmo;Subida total;Descida total;Potência média
Corrida;2026-07-15 06:30:00;Treino leve;5,20;360;00:31:12;142;171;164;06:00;05:10;45;40;230
`)

	activities, err := ParseCSV(input)
	if err != nil {
		t.Fatalf("ParseCSV returned error: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(activities))
	}

	activity := activities[0].Activity
	if activity.ActivityType != "corrida" {
		t.Errorf("expected activity type corrida, got %q", activity.ActivityType)
	}
	if activity.StartTime == nil || activity.StartTime.Format("2006-01-02 15:04:05") != "2026-07-15 06:30:00" {
		t.Fatalf("unexpected start time: %v", activity.StartTime)
	}
	if activity.DistanceMeters == nil || *activity.DistanceMeters != 5200 {
		t.Errorf("expected 5200m, got %v", activity.DistanceMeters)
	}
	if activity.DurationSeconds == nil || *activity.DurationSeconds != 1872 {
		t.Errorf("expected 1872s duration, got %v", activity.DurationSeconds)
	}
	if activity.AvgSpeedKMH == nil || *activity.AvgSpeedKMH != 10 {
		t.Errorf("expected 10 km/h average speed, got %v", activity.AvgSpeedKMH)
	}
	if activity.AnalyticsSummary == nil || activity.AnalyticsSummary.ActivitySummary.DistanceKM == nil {
		t.Fatalf("expected analytics summary")
	}
	if *activity.AnalyticsSummary.ActivitySummary.DistanceKM != 5.2 {
		t.Errorf("expected 5.2km summary, got %v", *activity.AnalyticsSummary.ActivitySummary.DistanceKM)
	}
}
