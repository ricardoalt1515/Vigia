-- name: ListBusinessDayHolidaysByVersion :many
SELECT calendar_version, holiday_date, label, source_note
FROM business_day_holidays
WHERE calendar_version = $1
ORDER BY holiday_date ASC;
