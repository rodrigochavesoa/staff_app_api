package garmin

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"staff_app/internal/domain"
)

// #nosec G101 - This is a map of Garmin CSV column headers, not hardcoded credentials.
var csvColumns = map[string]string{
	"Tipo de atividade":                 "activity_type",
	"Data":                              "start_time",
	"Título":                            "title",
	"Distância":                         "total_distance",
	"Calorias":                          "calories",
	"Tempo":                             "total_elapsed_time",
	"FC Média":                          "avg_heart_rate",
	"FC máxima":                         "max_heart_rate",
	"Cadência de corrida média":         "avg_cadence",
	"Cadência de corrida máxima":        "max_cadence",
	"Ritmo médio":                       "avg_pace",
	"Melhor ritmo":                      "best_pace",
	"Subida total":                      "elevation_gain",
	"Descida total":                     "elevation_loss",
	"Normalized Power® (NP®)":           "normalized_power",
	"Training Stress Score®":            "tss_score",
	"Potência média":                    "avg_power",
	"Energia máxima":                    "max_power",
	"TE Aeróbico":                       "aerobic_te",
	"Comprimento médio da passada":      "avg_stride_length",
	"Oscilação vertical média":          "avg_vertical_oscillation",
	"Proporção de média vertical":       "avg_vertical_ratio",
	"Tempo médio de contato com o solo": "avg_ground_contact_time",
}

type CSVActivity struct {
	Activity domain.GarminActivity
	Title    string
	AvgPace  string
	BestPace string
	Extra    map[string]any
}

func ParseCSV(r io.Reader) ([]CSVActivity, error) {
	data, err := io.ReadAll(io.LimitReader(r, 55*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading csv: %w", err)
	}

	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	if firstLine := firstNonEmptyLine(string(data)); strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		reader.Comma = ';'
	}

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parsing csv: %w", err)
	}
	if len(rows) < 2 {
		return nil, nil
	}

	headers := rows[0]
	var activities []CSVActivity
	for _, row := range rows[1:] {
		values := make(map[string]string, len(headers))
		for i, header := range headers {
			if i >= len(row) {
				continue
			}
			values[strings.TrimSpace(header)] = strings.TrimSpace(row[i])
		}
		activity := parseCSVRow(values)
		if activity.Activity.ActivityType == "" && activity.Activity.StartTime == nil && activity.Activity.DistanceMeters == nil {
			continue
		}
		activity.Activity.AnalyticsSummary = AnalyticsFromActivity(activity)
		activities = append(activities, activity)
	}

	return activities, nil
}

func parseCSVRow(row map[string]string) CSVActivity {
	a := CSVActivity{Extra: map[string]any{}}

	for csvName, field := range csvColumns {
		value := strings.TrimSpace(row[csvName])
		if value == "" {
			continue
		}

		switch field {
		case "activity_type":
			a.Activity.ActivityType = strings.ToLower(value)
		case "title":
			a.Title = value
		case "start_time":
			if t, ok := parseDateTime(value); ok {
				a.Activity.StartTime = &t
			}
		case "total_distance":
			if v, ok := parseFloat(value); ok {
				meters := v * 1000
				a.Activity.DistanceMeters = &meters
			}
		case "calories":
			if v, ok := parseInt(value); ok {
				a.Activity.Calories = &v
			}
		case "total_elapsed_time":
			if seconds, ok := parseDuration(value); ok {
				v := float64(seconds)
				a.Activity.DurationSeconds = &v
			}
		case "avg_heart_rate":
			if v, ok := parseFloat(value); ok {
				a.Activity.AvgHeartRate = &v
			}
		case "max_heart_rate":
			if v, ok := parseFloat(value); ok {
				a.Activity.MaxHeartRate = &v
			}
		case "avg_cadence":
			if v, ok := parseFloat(value); ok {
				a.Activity.AvgCadence = &v
			}
		case "elevation_gain":
			if v, ok := parseFloat(value); ok {
				a.Activity.ElevationGainM = &v
			}
		case "elevation_loss":
			if v, ok := parseFloat(value); ok {
				a.Activity.ElevationLossM = &v
			}
		case "avg_power":
			if v, ok := parseFloat(value); ok {
				a.Activity.AvgPowerWatts = &v
				a.Extra[field] = v
			}
		case "aerobic_te":
			if v, ok := parseFloat(value); ok {
				a.Activity.AerobicTE = &v
			}
		case "avg_pace":
			a.AvgPace = value
			if v, ok := paceToSpeed(value); ok {
				a.Activity.AvgSpeedKMH = &v
			}
		case "best_pace":
			a.BestPace = value
			if v, ok := paceToSpeed(value); ok {
				a.Activity.MaxSpeedKMH = &v
			}
		default:
			if v, ok := parseFloat(value); ok {
				a.Extra[field] = v
			} else {
				a.Extra[field] = value
			}
		}
	}

	return a
}

func AnalyticsFromActivity(a CSVActivity) *domain.AnalyticsSummary {
	activity := a.Activity
	var durationMinutes *float64
	if activity.DurationSeconds != nil {
		v := *activity.DurationSeconds / 60
		durationMinutes = &v
	}

	var distanceKM *float64
	if activity.DistanceMeters != nil {
		v := *activity.DistanceMeters / 1000
		distanceKM = &v
	}

	return &domain.AnalyticsSummary{
		ActivitySummary: domain.ActivitySummaryMetrics{
			Type:            activity.ActivityType,
			Title:           a.Title,
			StartTime:       activity.StartTime,
			DurationMinutes: durationMinutes,
			DistanceKM:      distanceKM,
			CaloriesBurned:  activity.Calories,
		},
		HeartRate: domain.HeartRateMetrics{
			Average: activity.AvgHeartRate,
			Maximum: activity.MaxHeartRate,
		},
		Cadence: domain.CadenceMetrics{
			Average: activity.AvgCadence,
		},
		Speed: domain.SpeedMetrics{
			Average: activity.AvgSpeedKMH,
			Maximum: activity.MaxSpeedKMH,
		},
		Elevation: domain.ElevationMetrics{
			TotalGainMeters: activity.ElevationGainM,
			TotalLossMeters: activity.ElevationLossM,
		},
		Power: domain.PowerMetrics{
			AvgPower: activity.AvgPowerWatts,
		},
	}
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

func parseDateTime(value string) (time.Time, bool) {
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"02/01/2006 15:04:05",
		"02/01/2006 15:04",
	}
	for _, layout := range layouts {
		t, err := time.Parse(layout, strings.TrimSpace(value))
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseDuration(value string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	switch len(parts) {
	case 3:
		h, errH := strconv.Atoi(parts[0])
		m, errM := strconv.Atoi(parts[1])
		s, errS := strconv.Atoi(parts[2])
		if errH == nil && errM == nil && errS == nil {
			return h*3600 + m*60 + s, true
		}
	case 2:
		m, errM := strconv.Atoi(parts[0])
		s, errS := strconv.Atoi(parts[1])
		if errM == nil && errS == nil {
			return m*60 + s, true
		}
	case 1:
		s, err := strconv.Atoi(parts[0])
		return s, err == nil
	}
	return 0, false
}

func parseFloat(value string) (float64, bool) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(value), ",", ".")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	v, err := strconv.ParseFloat(cleaned, 64)
	return v, err == nil
}

func parseInt(value string) (int, bool) {
	v, ok := parseFloat(value)
	return int(v), ok
}

func paceToSpeed(value string) (float64, bool) {
	seconds, ok := parseDuration(value)
	if !ok || seconds <= 0 {
		return 0, false
	}
	return 3600 / float64(seconds), true
}
