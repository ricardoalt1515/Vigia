package orchestrator

import (
	"testing"
	"time"

	"github.com/golang-sql/civil"
)

func TestAddBusinessDaysSkipsWeekends(t *testing.T) {
	cal := Calendar{Version: "mx-lft-art-74-2026a", Holidays: map[civil.Date]struct{}{}}
	from := time.Date(2026, time.January, 8, 9, 30, 0, 0, time.UTC) // Thursday

	due := AddBusinessDays(from, 10, cal)

	want := time.Date(2026, time.January, 22, 9, 30, 0, 0, time.UTC)
	if !due.Equal(want) {
		t.Fatalf("due = %s, want %s", due, want)
	}
}

func TestAddBusinessDaysSkipsSeededHoliday(t *testing.T) {
	cal := Calendar{
		Version: "mx-lft-art-74-2026a",
		Holidays: map[civil.Date]struct{}{
			civil.DateOf(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)): {},
		},
	}
	from := time.Date(2025, time.December, 31, 12, 0, 0, 0, time.UTC)

	due := AddBusinessDays(from, 1, cal)

	want := time.Date(2026, time.January, 2, 12, 0, 0, 0, time.UTC)
	if !due.Equal(want) {
		t.Fatalf("due = %s, want %s", due, want)
	}
}

func TestAddBusinessDaysTreatsUnseededAmbiguousDayAsBusinessDay(t *testing.T) {
	cal := Calendar{Version: "mx-lft-art-74-2026a", Holidays: map[civil.Date]struct{}{}}
	from := time.Date(2026, time.May, 4, 8, 0, 0, 0, time.UTC) // Monday; May 5 is intentionally not seeded here.

	due := AddBusinessDays(from, 1, cal)

	want := time.Date(2026, time.May, 5, 8, 0, 0, 0, time.UTC)
	if !due.Equal(want) {
		t.Fatalf("due = %s, want %s", due, want)
	}
}

func TestLoadCalendarPinsVersionAndHolidays(t *testing.T) {
	rows := []HolidayRow{
		{Version: "mx-lft-art-74-2026a", Date: civil.Date{Year: 2026, Month: time.January, Day: 1}},
		{Version: "other-version", Date: civil.Date{Year: 2026, Month: time.February, Day: 2}},
	}

	cal := LoadCalendar("mx-lft-art-74-2026a", rows)

	if cal.Version != "mx-lft-art-74-2026a" {
		t.Fatalf("version = %q", cal.Version)
	}
	if _, ok := cal.Holidays[civil.Date{Year: 2026, Month: time.January, Day: 1}]; !ok {
		t.Fatal("expected matching-version holiday to be loaded")
	}
	if _, ok := cal.Holidays[civil.Date{Year: 2026, Month: time.February, Day: 2}]; ok {
		t.Fatal("did not expect other-version holiday to be loaded")
	}
}
