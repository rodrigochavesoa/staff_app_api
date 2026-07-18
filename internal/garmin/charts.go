package garmin

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"staff_app/internal/domain"
)

type plotlyFigure struct {
	Data   []map[string]any `json:"data"`
	Layout map[string]any   `json:"layout"`
}

func ChartDistanceTimeline(activities []*domain.GarminActivity) (string, bool) {
	if len(activities) == 0 {
		return "", false
	}
	sorted := append([]*domain.GarminActivity(nil), activities...)
	sort.Slice(sorted, func(i, j int) bool {
		return timeValue(sorted[i].StartTime).Before(timeValue(sorted[j].StartTime))
	})

	var dates []string
	var distances []float64
	for _, activity := range sorted {
		if activity.StartTime == nil || activity.DistanceMeters == nil {
			continue
		}
		dates = append(dates, activity.StartTime.Format("2006-01-02"))
		distances = append(distances, round2(*activity.DistanceMeters/1000))
	}
	if len(dates) == 0 {
		return "", false
	}

	return mustChart(plotlyFigure{
		Data: []map[string]any{{
			"type":      "scatter",
			"mode":      "lines+markers",
			"name":      "Distância por Atividade",
			"x":         dates,
			"y":         distances,
			"line":      map[string]any{"color": "#2E55F0", "width": 3},
			"fill":      "tozeroy",
			"fillcolor": "rgba(46,85,240,0.12)",
		}},
		Layout: baseLayout("Distância ao Longo do Tempo", "Data", "Distância (km)"),
	}), true
}

func ChartActivityTypes(activities []*domain.GarminActivity) (string, bool) {
	if len(activities) == 0 {
		return "", false
	}
	counts := map[string]int{}
	for _, activity := range activities {
		t := activity.ActivityType
		if t == "" {
			t = "Outro"
		}
		counts[t]++
	}
	if len(counts) == 0 {
		return "", false
	}

	types := make([]string, 0, len(counts))
	for t := range counts {
		types = append(types, t)
	}
	sort.Strings(types)
	values := make([]int, 0, len(types))
	for _, t := range types {
		values = append(values, counts[t])
	}

	return mustChart(plotlyFigure{
		Data: []map[string]any{{
			"type":   "bar",
			"name":   "Quantidade",
			"x":      types,
			"y":      values,
			"marker": map[string]any{"color": chartColors(len(types))},
		}},
		Layout: baseLayout("Tipos de Atividades", "Tipo de Atividade", "Quantidade"),
	}), true
}

func ChartVelocityScatter(activities []*domain.GarminActivity) (string, bool) {
	var durations []float64
	var speeds []float64
	var labels []string
	for _, activity := range activities {
		if activity.DurationSeconds == nil || activity.DistanceMeters == nil || *activity.DurationSeconds <= 0 || *activity.DistanceMeters <= 0 {
			continue
		}
		durations = append(durations, *activity.DurationSeconds)
		speed := (*activity.DistanceMeters / 1000) / (*activity.DurationSeconds / 3600)
		speeds = append(speeds, round2(speed))
		labels = append(labels, fmt.Sprintf("%s %.2f km", nonEmpty(activity.ActivityType, "Atividade"), *activity.DistanceMeters/1000))
	}
	if len(durations) == 0 {
		return "", false
	}

	return mustChart(plotlyFigure{
		Data: []map[string]any{{
			"type":          "scatter",
			"mode":          "markers",
			"x":             durations,
			"y":             speeds,
			"text":          labels,
			"hovertemplate": "%{text}<br>Duração: %{x:.0f}s<br>Velocidade: %{y:.1f} km/h<extra></extra>",
			"marker":        map[string]any{"color": speeds, "colorscale": "Viridis", "showscale": true, "size": 10},
		}},
		Layout: baseLayout("Velocidade vs Duração", "Duração (s)", "Velocidade Média (km/h)"),
	}), true
}

func ChartHeartRateZones(samples []domain.HeartRateSample, fcmax int) (string, bool) {
	if len(samples) == 0 {
		return "", false
	}
	if fcmax <= 0 {
		fcmax = 190
	}

	counts := []int{0, 0, 0, 0, 0}
	for _, sample := range samples {
		if sample.HeartRate <= 0 {
			continue
		}
		pct := sample.HeartRate / float64(fcmax)
		switch {
		case pct < 0.60:
			counts[0]++
		case pct < 0.70:
			counts[1]++
		case pct < 0.80:
			counts[2]++
		case pct < 0.90:
			counts[3]++
		default:
			counts[4]++
		}
	}
	if sumInts(counts) == 0 {
		return "", false
	}

	labels := []string{
		fmt.Sprintf("Zona 1 %d-%d bpm", int(0.50*float64(fcmax)), int(0.60*float64(fcmax))-1),
		fmt.Sprintf("Zona 2 %d-%d bpm", int(0.60*float64(fcmax)), int(0.70*float64(fcmax))-1),
		fmt.Sprintf("Zona 3 %d-%d bpm", int(0.70*float64(fcmax)), int(0.80*float64(fcmax))-1),
		fmt.Sprintf("Zona 4 %d-%d bpm", int(0.80*float64(fcmax)), int(0.90*float64(fcmax))-1),
		fmt.Sprintf("Zona 5 > %d bpm", int(0.90*float64(fcmax))),
	}

	return mustChart(plotlyFigure{
		Data: []map[string]any{{
			"type":        "bar",
			"orientation": "h",
			"y":           labels,
			"x":           counts,
			"marker":      map[string]any{"color": []string{"#94A3B8", "#3B82F6", "#22C55E", "#F97316", "#EF4444"}},
		}},
		Layout: baseLayout("Zonas de Frequência Cardíaca", "Registros", ""),
	}), true
}

func ChartHRSeries(samples []domain.HeartRateSample) (string, bool) {
	if len(samples) == 0 {
		return "", false
	}
	if len(samples) > 500 {
		step := int(math.Ceil(float64(len(samples)) / 500))
		filtered := make([]domain.HeartRateSample, 0, 500)
		for i := 0; i < len(samples); i += step {
			filtered = append(filtered, samples[i])
		}
		samples = filtered
	}

	var x []string
	var y []float64
	for i, sample := range samples {
		if sample.HeartRate <= 0 {
			continue
		}
		if sample.Timestamp != nil {
			x = append(x, sample.Timestamp.Format(time.RFC3339))
		} else {
			x = append(x, fmt.Sprintf("%04ds", i))
		}
		y = append(y, sample.HeartRate)
	}
	if len(y) == 0 {
		return "", false
	}

	return mustChart(plotlyFigure{
		Data: []map[string]any{{
			"type":      "scatter",
			"mode":      "lines",
			"name":      "Frequência Cardíaca",
			"x":         x,
			"y":         y,
			"line":      map[string]any{"color": "#DC3545", "width": 2},
			"fill":      "tozeroy",
			"fillcolor": "rgba(220,53,69,0.10)",
		}},
		Layout: baseLayout("Série Temporal: Frequência Cardíaca", "Tempo", "FC (bpm)"),
	}), true
}

func ChartCalories(samples []domain.CaloriesSample) (string, bool) {
	if len(samples) == 0 {
		return "", false
	}
	var x []string
	var y []int
	for i, sample := range samples {
		if sample.Calories == nil {
			continue
		}
		if sample.ActivityDate != nil {
			x = append(x, sample.ActivityDate.Format("2006-01-02"))
		} else {
			x = append(x, fmt.Sprintf("Dia %d", i+1))
		}
		y = append(y, *sample.Calories)
	}
	if len(y) == 0 {
		return "", false
	}

	return mustChart(plotlyFigure{
		Data: []map[string]any{{
			"type":      "scatter",
			"mode":      "lines+markers",
			"name":      "Calorias",
			"x":         x,
			"y":         y,
			"fill":      "tozeroy",
			"fillcolor": "rgba(255,193,7,0.30)",
			"line":      map[string]any{"color": "#FFC107", "width": 2},
		}},
		Layout: baseLayout("Calorias Queimadas", "Data", "Calorias (kcal)"),
	}), true
}

func ChartDashboard(activities []*domain.GarminActivity, samples []domain.HeartRateSample, fcmax int) (string, bool) {
	distance, okDistance := ChartDistanceTimeline(activities)
	types, okTypes := ChartActivityTypes(activities)
	zones, okZones := ChartHeartRateZones(samples, fcmax)
	velocity, okVelocity := ChartVelocityScatter(activities)
	if !okDistance && !okTypes && !okZones && !okVelocity {
		return "", false
	}
	return mustChart(plotlyFigure{
		Data: []map[string]any{{
			"type": "dashboard_summary",
			"charts": map[string]any{
				"distance":          rawJSON(distance),
				"activity_types":    rawJSON(types),
				"heart_rate_zones":  rawJSON(zones),
				"velocity_scatter":  rawJSON(velocity),
				"distance_ok":       okDistance,
				"activity_types_ok": okTypes,
				"heart_rate_ok":     okZones,
				"velocity_ok":       okVelocity,
			},
		}},
		Layout: map[string]any{
			"title":  "Dashboard Garmin Completo",
			"height": 800,
		},
	}), true
}

func baseLayout(title, xTitle, yTitle string) map[string]any {
	return map[string]any{
		"title":         map[string]any{"text": title},
		"xaxis":         map[string]any{"title": xTitle},
		"yaxis":         map[string]any{"title": yTitle},
		"height":        400,
		"template":      "plotly_white",
		"showlegend":    true,
		"hovermode":     "x unified",
		"paper_bgcolor": "white",
		"plot_bgcolor":  "white",
	}
}

func mustChart(fig plotlyFigure) string {
	b, err := json.Marshal(fig)
	if err != nil {
		return `{"data":[],"layout":{}}`
	}
	return string(b)
}

func rawJSON(s string) any {
	if s == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	return v
}

func chartColors(n int) []string {
	palette := []string{"#2E55F0", "#17A2B8", "#28A745", "#FD7E14", "#6F42C1", "#E83E8C", "#20C997", "#FFC107"}
	out := make([]string, n)
	for i := range out {
		out[i] = palette[i%len(palette)]
	}
	return out
}

func timeValue(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func nonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
