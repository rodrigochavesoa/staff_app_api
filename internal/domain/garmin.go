package domain

import "time"

type GarminActivity struct {
	ID               int64             `json:"id"`
	AlunoID          int64             `json:"aluno_id"`
	FileNome         string            `json:"file_nome"`
	ActivityType     string            `json:"activity_type,omitempty"`
	StartTime        *time.Time        `json:"start_time,omitempty"`
	DurationSeconds  *float64          `json:"duration_seconds,omitempty"`
	DistanceMeters   *float64          `json:"distance_meters,omitempty"`
	ElevationGainM   *float64          `json:"elevation_gain_m,omitempty"`
	ElevationLossM   *float64          `json:"elevation_loss_m,omitempty"`
	Calories         *int              `json:"calories,omitempty"`
	AvgPowerWatts    *float64          `json:"avg_power_watts,omitempty"`
	ThresholdPower   *float64          `json:"threshold_power,omitempty"`
	AvgHeartRate     *float64          `json:"avg_heart_rate,omitempty"`
	MaxHeartRate     *float64          `json:"max_heart_rate,omitempty"`
	AvgCadence       *float64          `json:"avg_cadence,omitempty"`
	AvgSpeedKMH      *float64          `json:"avg_speed_kmh,omitempty"`
	MaxSpeedKMH      *float64          `json:"max_speed_kmh,omitempty"`
	AerobicTE        *float64          `json:"aerobic_te,omitempty"`
	AnaerobicTE      *float64          `json:"anaerobic_te,omitempty"`
	Records          []ActivityRecord  `json:"records,omitempty"`
	AnalyticsSummary *AnalyticsSummary `json:"analytics_summary,omitempty"`
}

type ActivityRecord struct {
	ID          int64      `json:"id,omitempty"`
	AtividadeID int64      `json:"atividade_id,omitempty"`
	Timestamp   *time.Time `json:"timestamp,omitempty"`
	Latitude    *float64   `json:"latitude,omitempty"`
	Longitude   *float64   `json:"longitude,omitempty"`
	AltitudeM   *float64   `json:"altitude_m,omitempty"`
	HeartRate   *int       `json:"heart_rate,omitempty"`
	Cadence     *int       `json:"cadence,omitempty"`
	SpeedKMH    *float64   `json:"speed_kmh,omitempty"`
	PowerWatts  *float64   `json:"power_watts,omitempty"`
	RawData     string     `json:"raw_data,omitempty"`
}

type GarminAnalytics struct {
	ID                   int64    `json:"id"`
	AtividadeID          int64    `json:"atividade_id"`
	TSSScore             *float64 `json:"tss_score,omitempty"`
	HeartRateVariability *float64 `json:"heart_rate_variability,omitempty"`
}

type AnalyticsSummary struct {
	ActivitySummary ActivitySummaryMetrics `json:"activity_summary"`
	HeartRate       HeartRateMetrics       `json:"heart_rate_metrics"`
	Cadence         CadenceMetrics         `json:"cadence_metrics"`
	Speed           SpeedMetrics           `json:"speed_metrics"`
	Elevation       ElevationMetrics       `json:"elevation_metrics"`
	Power           PowerMetrics           `json:"power_metrics"`
}

type ActivitySummaryMetrics struct {
	Type            string     `json:"type,omitempty"`
	Title           string     `json:"title,omitempty"`
	StartTime       *time.Time `json:"start_time,omitempty"`
	DurationMinutes *float64   `json:"duration_minutes,omitempty"`
	DistanceKM      *float64   `json:"distance_km,omitempty"`
	CaloriesBurned  *int       `json:"calories_burned,omitempty"`
}

type HeartRateMetrics struct {
	Average *float64 `json:"average,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`
	Min     *float64 `json:"min,omitempty"`
}

type CadenceMetrics struct {
	Average *float64 `json:"average,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`
}

type SpeedMetrics struct {
	Average *float64 `json:"average,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`
}

type ElevationMetrics struct {
	TotalGainMeters *float64 `json:"total_gain_meters,omitempty"`
	TotalLossMeters *float64 `json:"total_loss_meters,omitempty"`
	MinElevation    *float64 `json:"min_elevation,omitempty"`
	MaxElevation    *float64 `json:"max_elevation,omitempty"`
}

type PowerMetrics struct {
	AvgPower        *float64 `json:"avg_power,omitempty"`
	MaxPower        *float64 `json:"max_power,omitempty"`
	NormalizedPower *float64 `json:"normalized_power,omitempty"`
	TSSScore        *float64 `json:"tss_score,omitempty"`
	ThresholdPower  *float64 `json:"threshold_power,omitempty"`
}

type GarminStats struct {
	TotalActivities    int64                 `json:"total_activities"`
	TotalDurationHours float64               `json:"total_duration_hours"`
	TotalDistanceKM    float64               `json:"total_distance_km"`
	AvgHeartRate       *int                  `json:"avg_heart_rate,omitempty"`
	MaxHeartRate       *float64              `json:"max_heart_rate,omitempty"`
	LastActivity       *time.Time            `json:"last_activity,omitempty"`
	ActivitiesByType   []ActivityTypeSummary `json:"activities_by_type"`
}

type ActivityTypeSummary struct {
	Type               string  `json:"type"`
	Count              int64   `json:"count"`
	TotalDistanceKM    float64 `json:"total_distance_km"`
	TotalDurationHours float64 `json:"total_duration_hours"`
}

type HeartRateSample struct {
	HeartRate float64    `json:"avg_heart_rate"`
	Timestamp *time.Time `json:"timestamp,omitempty"`
	Estimated bool       `json:"__estimated__,omitempty"`
}

type CaloriesSample struct {
	Calories     *int       `json:"calories,omitempty"`
	AvgHeartRate *float64   `json:"avg_heart_rate,omitempty"`
	ActivityDate *time.Time `json:"activity_date,omitempty"`
}
