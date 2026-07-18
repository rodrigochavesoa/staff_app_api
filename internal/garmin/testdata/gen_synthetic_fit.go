//go:build ignore

// Generates a minimal synthetic FIT activity for unit/CI tests.
// Run from this directory:
//
//	go run gen_synthetic_fit.go
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tormoder/fit"
)

func main() {
	outPath := filepath.Join("fit", "synthetic_activity.fit")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fatal(err)
	}

	h := fit.NewHeader(fit.V20, false)
	file, err := fit.NewFile(fit.FileTypeActivity, h)
	if err != nil {
		fatal(err)
	}
	act, err := file.Activity()
	if err != nil {
		fatal(err)
	}

	start := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	ev := fit.NewEventMsg()
	ev.Timestamp = start
	ev.Event = fit.EventTimer
	ev.EventType = fit.EventTypeStart
	act.Events = append(act.Events, ev)

	session := fit.NewSessionMsg()
	session.Timestamp = start.Add(10 * time.Minute)
	session.StartTime = start
	session.Sport = fit.SportRunning
	session.SubSport = fit.SubSportGeneric
	// Scaled fields: elapsed time in milliseconds, distance in centimeters.
	session.TotalElapsedTime = 600 * 1000
	session.TotalTimerTime = 600 * 1000
	session.TotalDistance = 1000 * 100
	session.TotalCalories = 80
	session.AvgHeartRate = 140
	session.MaxHeartRate = 165
	act.Sessions = append(act.Sessions, session)

	// Records without GPS — heart rate + timestamps only (no real location data).
	for i := 0; i < 12; i++ {
		rec := fit.NewRecordMsg()
		rec.Timestamp = start.Add(time.Duration(i) * 50 * time.Second)
		rec.HeartRate = uint8(130 + i)
		act.Records = append(act.Records, rec)
	}

	f, err := os.Create(outPath)
	if err != nil {
		fatal(err)
	}
	defer f.Close()

	if err := fit.Encode(f, file, binary.LittleEndian); err != nil {
		fatal(err)
	}
	fmt.Println("wrote", outPath)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
