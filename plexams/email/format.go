package email

import (
	"fmt"
	"time"
)

// weekdayShortDE maps a time.Weekday (Sunday = 0) to its German two-letter abbreviation.
var weekdayShortDE = map[int]string{
	0: "So", 1: "Mo", 2: "Di", 3: "Mi", 4: "Do", 5: "Fr", 6: "Sa",
}

// WeekdayDE returns the German two-letter weekday abbreviation of t, e.g. "Mo".
func WeekdayDE(t time.Time) string {
	return weekdayShortDE[int(t.Weekday())]
}

// DateDE renders t as e.g. "Mo, 06.07.2026" — the German weekday abbreviation and the
// date. Used in the room/NTA/KDP/LBA overview emails.
func DateDE(t time.Time) string {
	return fmt.Sprintf("%s, %s", WeekdayDE(t), t.Format("02.01.2006"))
}

// TimeDE renders t's clock time as e.g. "08:30".
func TimeDE(t time.Time) string {
	return t.Format("15:04")
}
