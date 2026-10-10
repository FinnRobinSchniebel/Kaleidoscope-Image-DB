package imageset

import "time"

// dayOnlyTimeOfDay marks a stored source date whose time of day is unknown,
// e.g. a zip [Date]. No whole-second service timestamp can equal it, and
// MongoDB keeps its milliseconds.
const dayOnlyTimeOfDay = 17*time.Minute + 17*time.Millisecond

// DayOnlyDate returns day's UTC calendar day, marked as day-only.
func DayOnlyDate(day time.Time) time.Time {
	y, m, d := day.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Add(dayOnlyTimeOfDay)
}

func isDayOnly(t time.Time) bool {
	return t.Equal(DayOnlyDate(t))
}

// SourceDateMatches reports whether a stored source date still matches the
// service's date: by UTC calendar day when stored is day-only, exactly otherwise.
func SourceDateMatches(stored, service time.Time) bool {
	if isDayOnly(stored) {
		return DayOnlyDate(service).Equal(stored)
	}
	return stored.Equal(service)
}

// mergeSourceDate returns the date to store when incoming updates stored. An
// empty incoming date or the same instant keeps stored, and a day-only date
// never replaces an exact date on the same day.
func mergeSourceDate(stored, incoming time.Time) time.Time {
	switch {
	case incoming.IsZero(), incoming.Equal(stored):
		return stored
	case isDayOnly(incoming) && !isDayOnly(stored) && DayOnlyDate(stored).Equal(incoming):
		return stored
	default:
		return incoming
	}
}
