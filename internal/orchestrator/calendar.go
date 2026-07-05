package orchestrator

import (
	"time"

	"github.com/golang-sql/civil"
)

type Calendar struct {
	Version  string
	Holidays map[civil.Date]struct{}
}

type HolidayRow struct {
	Version string
	Date    civil.Date
}

func LoadCalendar(version string, rows []HolidayRow) Calendar {
	holidays := make(map[civil.Date]struct{})
	for _, row := range rows {
		if row.Version != version {
			continue
		}
		holidays[row.Date] = struct{}{}
	}
	return Calendar{Version: version, Holidays: holidays}
}

func AddBusinessDays(from time.Time, n int, cal Calendar) time.Time {
	if n <= 0 {
		return from
	}

	current := from
	remaining := n
	for remaining > 0 {
		current = current.AddDate(0, 0, 1)
		if isBusinessDay(current, cal) {
			remaining--
		}
	}
	return current
}

func isBusinessDay(day time.Time, cal Calendar) bool {
	weekday := day.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}
	_, isSeededHoliday := cal.Holidays[civil.DateOf(day)]
	return !isSeededHoliday
}
