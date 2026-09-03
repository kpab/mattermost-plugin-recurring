package reminder

import "time"

// maxLookaheadDays bounds the search for the next firing day. Every recurring
// kind we support fires at least once in any 62-day window (the longest gap is
// a monthly reminder on the 31st landing after a short month), so a schedule
// that finds nothing within this window is one we cannot satisfy.
const maxLookaheadDays = 400

// NextRun returns the first time strictly after `after` at which the schedule
// fires. The wall-clock time of day is interpreted in loc, so a reminder set
// for 09:00 stays at 09:00 across a daylight-saving change rather than
// drifting to 08:00 or 10:00.
//
// The second return value is false when the schedule will never fire again,
// which for a one-off reminder simply means its time has passed.
func (s Schedule) NextRun(after time.Time, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = time.UTC
	}

	if s.Kind == KindOnce {
		at := time.UnixMilli(s.OnceAt).In(loc)
		if at.After(after) {
			return at, true
		}
		return time.Time{}, false
	}

	// Walk forward a day at a time from the day `after` falls on.
	//
	// The days are enumerated as plain calendar dates rather than as timestamps
	// in loc. Stepping a timestamp would drop a day outright in the zones whose
	// clocks jump at midnight — America/Santiago goes 00:00 to 01:00 each
	// September, so the local midnight of that day does not exist and
	// normalising it lands on the previous date. A weekly reminder would skip a
	// week and a monthly one a whole month.
	year, month, day := after.In(loc).Date()

	for i := range maxLookaheadDays {
		y, m, d := addDays(year, month, day, i)
		if !s.fallsOn(y, m, d) {
			continue
		}

		candidate := s.timeOn(y, m, d, loc)
		if candidate.After(after) {
			return candidate, true
		}
	}

	return time.Time{}, false
}

// addDays advances a calendar date by n days. It normalises through UTC, which
// has no offset transitions, so the arithmetic is pure date arithmetic and
// cannot be perturbed by the reminder's own timezone.
func addDays(year int, month time.Month, day, n int) (int, time.Month, int) {
	t := time.Date(year, month, day+n, 12, 0, 0, 0, time.UTC)
	return t.Year(), t.Month(), t.Day()
}

// weekdayOn returns the day of the week a calendar date falls on. Noon UTC is
// used purely as a stable point inside the day.
func weekdayOn(year int, month time.Month, day int) time.Weekday {
	return time.Date(year, month, day, 12, 0, 0, 0, time.UTC).Weekday()
}

// timeOn places the schedule's time of day onto the given calendar date.
//
// Around a spring-forward transition the requested wall-clock time may not
// exist, and time.Date resolves it to a real instant that reads back as a
// different wall clock: usually an hour earlier (02:30 becomes 01:30 in
// America/New_York), and an hour later in zones that jump at midnight. Firing
// early is the worse failure, so the result is nudged onto the far side of the
// gap, which is what calendar apps do.
func (s Schedule) timeOn(year int, month time.Month, day int, loc *time.Location) time.Time {
	candidate := time.Date(year, month, day, s.At.Hour, s.At.Minute, 0, 0, loc)

	wanted := s.At.Hour*60 + s.At.Minute
	got := candidate.Hour()*60 + candidate.Minute()
	if got < wanted {
		candidate = candidate.Add(time.Duration(wanted-got) * time.Minute)
	}

	return candidate
}

// fallsOn reports whether the schedule fires on the given calendar date,
// ignoring the time of day.
func (s Schedule) fallsOn(year int, month time.Month, day int) bool {
	switch s.Kind {
	case KindDaily:
		return true

	case KindWeekdays:
		weekday := weekdayOn(year, month, day)
		return weekday != time.Saturday && weekday != time.Sunday

	case KindWeekly:
		weekday := weekdayOn(year, month, day)
		for _, d := range s.Weekdays {
			if d == weekday {
				return true
			}
		}
		return false

	case KindMonthly:
		return day == effectiveDayOfMonth(s.DayOfMonth, year, month)

	case KindOnce:
		// Handled in NextRun, which never reaches here.
		return false

	default:
		return false
	}
}

// effectiveDayOfMonth clamps a day-of-month to the length of the given month,
// so a reminder set for the 31st fires on the last day of February rather than
// being skipped for four months of the year.
func effectiveDayOfMonth(dayOfMonth, year int, month time.Month) int {
	if last := daysInMonth(year, month); dayOfMonth > last {
		return last
	}
	return dayOfMonth
}

// daysInMonth returns the number of days in the given month.
func daysInMonth(year int, month time.Month) int {
	// Day 0 of the following month is the last day of this one.
	return time.Date(year, month+1, 0, 12, 0, 0, 0, time.UTC).Day()
}

// NextRun returns the reminder's next firing time, in its own timezone.
func (r *Reminder) NextRun(after time.Time) (time.Time, bool) {
	return r.Schedule.NextRun(after, r.Location())
}

// Advance moves NextRunAt to the next firing time after `after`, and reports
// whether the reminder is still live. A reminder that will not fire again gets
// a zero NextRunAt so the scheduler knows not to queue it.
func (r *Reminder) Advance(after time.Time) bool {
	next, ok := r.NextRun(after)
	if !ok {
		r.NextRunAt = 0
		return false
	}

	r.NextRunAt = next.UnixMilli()

	return true
}
