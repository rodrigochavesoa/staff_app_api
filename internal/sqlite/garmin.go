package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"staff_app/internal/domain"
)

type GarminRepository struct {
	db *DB
}

func NewGarminRepository(db *DB) *GarminRepository {
	return &GarminRepository{db: db}
}

func (r *GarminRepository) SaveActivity(ctx context.Context, activity *domain.GarminActivity) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("starting garmin transaction: %w", err)
	}
	defer tx.Rollback()

	if activity.StartTime != nil && activity.DistanceMeters != nil {
		var existingID int64
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM atividades_garmin
			WHERE aluno_id = ?
			  AND start_time = ?
			  AND ABS(COALESCE(distance_meters, 0) - ?) < 10
			LIMIT 1
		`, activity.AlunoID, formatTime(activity.StartTime), *activity.DistanceMeters).Scan(&existingID)
		if err == nil {
			return 0, fmt.Errorf("atividade já registrada (id=%d, start_time=%s)", existingID, formatTime(activity.StartTime))
		}
		if err != nil && err != sql.ErrNoRows {
			return 0, fmt.Errorf("checking duplicate garmin activity: %w", err)
		}
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO atividades_garmin (
			aluno_id, file_nome, activity_type,
			start_time, duration_seconds, distance_meters,
			elevation_gain_m, elevation_loss_m,
			calories, avg_power_watts, threshold_power,
			avg_heart_rate, max_heart_rate,
			avg_cadence, avg_speed_kmh, max_speed_kmh,
			aerobic_te, anaerobic_te
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		activity.AlunoID,
		activity.FileNome,
		nullString(activity.ActivityType),
		formatTime(activity.StartTime),
		nullFloat(activity.DurationSeconds),
		nullFloat(activity.DistanceMeters),
		nullFloat(activity.ElevationGainM),
		nullFloat(activity.ElevationLossM),
		nullInt(activity.Calories),
		nullFloat(activity.AvgPowerWatts),
		nullFloat(activity.ThresholdPower),
		nullFloat(activity.AvgHeartRate),
		nullFloat(activity.MaxHeartRate),
		nullFloat(activity.AvgCadence),
		nullFloat(activity.AvgSpeedKMH),
		nullFloat(activity.MaxSpeedKMH),
		nullFloat(activity.AerobicTE),
		nullFloat(activity.AnaerobicTE),
	)
	if err != nil {
		return 0, fmt.Errorf("inserting garmin activity: %w", err)
	}

	activityID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading garmin activity id: %w", err)
	}
	activity.ID = activityID

	if err := insertGarminActivityRecords(ctx, tx, activityID, activity.Records); err != nil {
		return 0, err
	}

	if activity.AnalyticsSummary != nil {
		_, err = tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO atividades_analytics (
				atividade_id,
				tss_score,
				heart_rate_variability
			) VALUES (?, ?, ?)
		`, activityID, nullFloat(activity.AnalyticsSummary.Power.TSSScore), nil)
		if err != nil {
			return 0, fmt.Errorf("inserting garmin analytics: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing garmin transaction: %w", err)
	}
	return activityID, nil
}

func (r *GarminRepository) Activity(ctx context.Context, id int64) (*domain.GarminActivity, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, aluno_id, file_nome, activity_type, start_time, duration_seconds, distance_meters,
		       elevation_gain_m, elevation_loss_m, calories, avg_power_watts, threshold_power,
		       avg_heart_rate, max_heart_rate, avg_cadence, avg_speed_kmh, max_speed_kmh,
		       aerobic_te, anaerobic_te
		FROM atividades_garmin
		WHERE id = ?
	`, id)
	return scanActivity(row)
}

func (r *GarminRepository) ActivityRecords(ctx context.Context, activityID int64, limit int) ([]domain.ActivityRecord, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, atividade_id, timestamp, latitude, longitude, altitude_m,
		       heart_rate, cadence, speed_kmh, power_watts, raw_data
		FROM atividades_records
		WHERE atividade_id = ?
		ORDER BY timestamp ASC
		LIMIT ?
	`, activityID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying garmin records: %w", err)
	}
	defer rows.Close()

	var records []domain.ActivityRecord
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	return records, rows.Err()
}

func (r *GarminRepository) ActivityAnalytics(ctx context.Context, activityID int64) (*domain.GarminAnalytics, error) {
	var analytics domain.GarminAnalytics
	var tss, hrv sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `
		SELECT atividade_id, tss_score, heart_rate_variability
		FROM atividades_analytics
		WHERE atividade_id = ?
	`, activityID).Scan(&analytics.AtividadeID, &tss, &hrv)
	if err != nil {
		return nil, err
	}
	analytics.ID = analytics.AtividadeID
	analytics.TSSScore = ptrNullFloat(tss)
	analytics.HeartRateVariability = ptrNullFloat(hrv)
	return &analytics, nil
}

func (r *GarminRepository) ListAlunoActivities(ctx context.Context, alunoID int64, activityType string, limit, offset int) ([]*domain.GarminActivity, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	countQuery := "SELECT COUNT(*) FROM atividades_garmin WHERE aluno_id = ?"
	query := `
		SELECT id, aluno_id, file_nome, activity_type, start_time, duration_seconds, distance_meters,
		       elevation_gain_m, elevation_loss_m, calories, avg_power_watts, threshold_power,
		       avg_heart_rate, max_heart_rate, avg_cadence, avg_speed_kmh, max_speed_kmh,
		       aerobic_te, anaerobic_te
		FROM atividades_garmin
		WHERE aluno_id = ?
	`
	args := []any{alunoID}
	if activityType != "" {
		countQuery += " AND activity_type = ?"
		query += " AND activity_type = ?"
		args = append(args, activityType)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting garmin activities: %w", err)
	}

	query += " ORDER BY start_time DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying garmin activities: %w", err)
	}
	defer rows.Close()

	var activities []*domain.GarminActivity
	for rows.Next() {
		activity, err := scanActivity(rows)
		if err != nil {
			return nil, 0, err
		}
		activities = append(activities, activity)
	}
	return activities, total, rows.Err()
}

func (r *GarminRepository) AlunoStats(ctx context.Context, alunoID int64) (*domain.GarminStats, error) {
	var stats domain.GarminStats
	var totalSeconds, totalDistance, avgHR, maxHR sql.NullFloat64
	var lastActivity sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*), SUM(duration_seconds), SUM(distance_meters),
		       AVG(avg_heart_rate), MAX(max_heart_rate), MAX(start_time)
		FROM atividades_garmin
		WHERE aluno_id = ?
	`, alunoID).Scan(&stats.TotalActivities, &totalSeconds, &totalDistance, &avgHR, &maxHR, &lastActivity)
	if err != nil {
		return nil, fmt.Errorf("querying garmin stats: %w", err)
	}
	stats.TotalDurationHours = round2(totalSeconds.Float64 / 3600)
	stats.TotalDistanceKM = round2(totalDistance.Float64 / 1000)
	if avgHR.Valid {
		v := int(math.Round(avgHR.Float64))
		stats.AvgHeartRate = &v
	}
	stats.MaxHeartRate = ptrNullFloat(maxHR)
	if lastActivity.Valid {
		stats.LastActivity = parseDBTime(lastActivity.String)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT activity_type, COUNT(*), SUM(distance_meters), SUM(duration_seconds)
		FROM atividades_garmin
		WHERE aluno_id = ?
		GROUP BY activity_type
		ORDER BY COUNT(*) DESC
	`, alunoID)
	if err != nil {
		return nil, fmt.Errorf("querying garmin stats by type: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item domain.ActivityTypeSummary
		var distance, duration sql.NullFloat64
		if err := rows.Scan(&item.Type, &item.Count, &distance, &duration); err != nil {
			return nil, err
		}
		item.TotalDistanceKM = round2(distance.Float64 / 1000)
		item.TotalDurationHours = round2(duration.Float64 / 3600)
		stats.ActivitiesByType = append(stats.ActivitiesByType, item)
	}
	return &stats, rows.Err()
}

func (r *GarminRepository) DeleteActivity(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM atividades_garmin WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting garmin activity: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *GarminRepository) MaxHeartRate(ctx context.Context, alunoID int64) (int, error) {
	var maxHR sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `
		SELECT MAX(max_heart_rate)
		FROM atividades_garmin
		WHERE aluno_id = ?
	`, alunoID).Scan(&maxHR)
	if err != nil {
		return 0, fmt.Errorf("querying max heart rate: %w", err)
	}
	if !maxHR.Valid || maxHR.Float64 <= 0 {
		return 190, nil
	}
	return int(math.Round(maxHR.Float64)), nil
}

func (r *GarminRepository) HeartRateSamples(ctx context.Context, alunoID int64, limit int) ([]domain.HeartRateSample, bool, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT r.heart_rate, r.timestamp
		FROM atividades_records r
		JOIN atividades_garmin a ON r.atividade_id = a.id
		WHERE a.aluno_id = ? AND r.heart_rate IS NOT NULL
		ORDER BY r.timestamp
		LIMIT ?
	`, alunoID, limit)
	if err != nil {
		return nil, false, fmt.Errorf("querying heart rate records: %w", err)
	}
	samples, err := scanHeartRateSamples(rows, false)
	if err != nil {
		return nil, false, err
	}
	if len(samples) > 0 {
		return samples, false, nil
	}

	rows, err = r.db.QueryContext(ctx, `
		SELECT avg_heart_rate, start_time
		FROM atividades_garmin
		WHERE aluno_id = ? AND avg_heart_rate IS NOT NULL
		ORDER BY start_time
		LIMIT ?
	`, alunoID, limit)
	if err != nil {
		return nil, false, fmt.Errorf("querying estimated heart rate samples: %w", err)
	}
	samples, err = scanHeartRateSamples(rows, true)
	return samples, true, err
}

func (r *GarminRepository) CaloriesSamples(ctx context.Context, alunoID int64, limit int) ([]domain.CaloriesSample, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT calories, avg_heart_rate, start_time
		FROM atividades_garmin
		WHERE aluno_id = ?
		ORDER BY start_time
		LIMIT ?
	`, alunoID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying calories samples: %w", err)
	}
	defer rows.Close()

	var samples []domain.CaloriesSample
	for rows.Next() {
		var calories sql.NullInt64
		var avgHR sql.NullFloat64
		var startTime sql.NullString
		if err := rows.Scan(&calories, &avgHR, &startTime); err != nil {
			return nil, err
		}
		var sample domain.CaloriesSample
		if calories.Valid {
			v := int(calories.Int64)
			sample.Calories = &v
		}
		sample.AvgHeartRate = ptrNullFloat(avgHR)
		if startTime.Valid {
			sample.ActivityDate = parseDBTime(startTime.String)
		}
		if sample.Calories != nil || sample.AvgHeartRate != nil {
			samples = append(samples, sample)
		}
	}
	return samples, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanActivity(row scanner) (*domain.GarminActivity, error) {
	var activity domain.GarminActivity
	var activityType, startTime sql.NullString
	var duration, distance, gain, loss, avgPower, thresholdPower, avgHR, maxHR, avgCadence, avgSpeed, maxSpeed, aerobic, anaerobic sql.NullFloat64
	var calories sql.NullInt64
	err := row.Scan(
		&activity.ID,
		&activity.AlunoID,
		&activity.FileNome,
		&activityType,
		&startTime,
		&duration,
		&distance,
		&gain,
		&loss,
		&calories,
		&avgPower,
		&thresholdPower,
		&avgHR,
		&maxHR,
		&avgCadence,
		&avgSpeed,
		&maxSpeed,
		&aerobic,
		&anaerobic,
	)
	if err != nil {
		return nil, err
	}
	if activityType.Valid {
		activity.ActivityType = activityType.String
	}
	activity.StartTime = parseDBTime(startTime.String)
	activity.DurationSeconds = ptrNullFloat(duration)
	activity.DistanceMeters = ptrNullFloat(distance)
	activity.ElevationGainM = ptrNullFloat(gain)
	activity.ElevationLossM = ptrNullFloat(loss)
	if calories.Valid {
		v := int(calories.Int64)
		activity.Calories = &v
	}
	activity.AvgPowerWatts = ptrNullFloat(avgPower)
	activity.ThresholdPower = ptrNullFloat(thresholdPower)
	activity.AvgHeartRate = ptrNullFloat(avgHR)
	activity.MaxHeartRate = ptrNullFloat(maxHR)
	activity.AvgCadence = ptrNullFloat(avgCadence)
	activity.AvgSpeedKMH = ptrNullFloat(avgSpeed)
	activity.MaxSpeedKMH = ptrNullFloat(maxSpeed)
	activity.AerobicTE = ptrNullFloat(aerobic)
	activity.AnaerobicTE = ptrNullFloat(anaerobic)
	return &activity, nil
}

func scanRecord(rows *sql.Rows) (*domain.ActivityRecord, error) {
	var record domain.ActivityRecord
	var ts, rawData sql.NullString
	var latitude, longitude, altitude, speed, power sql.NullFloat64
	var hr, cadence sql.NullInt64
	err := rows.Scan(&record.ID, &record.AtividadeID, &ts, &latitude, &longitude, &altitude, &hr, &cadence, &speed, &power, &rawData)
	if err != nil {
		return nil, err
	}
	record.Timestamp = parseDBTime(ts.String)
	record.Latitude = ptrNullFloat(latitude)
	record.Longitude = ptrNullFloat(longitude)
	record.AltitudeM = ptrNullFloat(altitude)
	if hr.Valid {
		v := int(hr.Int64)
		record.HeartRate = &v
	}
	if cadence.Valid {
		v := int(cadence.Int64)
		record.Cadence = &v
	}
	record.SpeedKMH = ptrNullFloat(speed)
	record.PowerWatts = ptrNullFloat(power)
	if rawData.Valid {
		record.RawData = rawData.String
	}
	return &record, nil
}

func scanHeartRateSamples(rows *sql.Rows, estimated bool) ([]domain.HeartRateSample, error) {
	defer rows.Close()
	var samples []domain.HeartRateSample
	for rows.Next() {
		var hr sql.NullFloat64
		var ts sql.NullString
		if err := rows.Scan(&hr, &ts); err != nil {
			return nil, err
		}
		if !hr.Valid || hr.Float64 <= 0 {
			continue
		}
		sample := domain.HeartRateSample{
			HeartRate: hr.Float64,
			Estimated: estimated,
		}
		if ts.Valid {
			sample.Timestamp = parseDBTime(ts.String)
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func formatTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

func parseDBTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05"}
	for _, layout := range layouts {
		t, err := time.Parse(layout, value)
		if err == nil {
			return &t
		}
	}
	return nil
}

func ptrNullFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

const garminRecordInsertBatchSize = 100

func insertGarminActivityRecords(ctx context.Context, tx *sql.Tx, activityID int64, records []domain.ActivityRecord) error {
	if len(records) == 0 {
		return nil
	}

	const insertSQL = `
		INSERT INTO atividades_records (
			atividade_id, timestamp,
			latitude, longitude, altitude_m,
			heart_rate, cadence, speed_kmh,
			power_watts, raw_data
		) VALUES `

	for start := 0; start < len(records); start += garminRecordInsertBatchSize {
		end := start + garminRecordInsertBatchSize
		if end > len(records) {
			end = len(records)
		}
		chunk := records[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, 0, len(chunk)*10)
		for i, record := range chunk {
			rawData := record.RawData
			if rawData == "" {
				if b, err := json.Marshal(record); err == nil {
					rawData = string(b)
				}
			}
			placeholders[i] = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
			args = append(args,
				activityID,
				formatTime(record.Timestamp),
				nullFloat(record.Latitude),
				nullFloat(record.Longitude),
				nullFloat(record.AltitudeM),
				nullInt(record.HeartRate),
				nullInt(record.Cadence),
				nullFloat(record.SpeedKMH),
				nullFloat(record.PowerWatts),
				nullString(rawData),
			)
		}

		query := insertSQL + strings.Join(placeholders, ", ")
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("inserting garmin records batch: %w", err)
		}
	}
	return nil
}
