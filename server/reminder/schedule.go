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

	// Walk forward a day at a time from the day `after` falls on. Working in
	// whole days and rebuilding the timestamp with time.Date keeps this correct
	// across daylight-saving transitions, where adding 24h would not be.
	start := after.In(loc)
	cursor := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)

	for i := range maxLookaheadDays {
		day := cursor.AddDate(0, 0, i)
		if !s.fallsOn(day) {
			continue
		}

		candidate := s.timeOn(day, loc)
		if candidate.After(after) {
			return candidate, true
		}
	}

	return time.Time{}, false
}

// timeOn places the schedule's time of day onto the given day.
//
// On the day a timezone springs forward, the requested wall-clock time may not
// exist. time.Date resolves such a time backwards — asking for 02:30 on a day
// that jumps 02:00 to 03:00 yields 01:30, an hour *earlier* than asked for.
// Firing early is the wrong way to be wrong, so we push the reminder forward to
// the far side of the gap instead, matching what calendar apps do.
func (s Schedule) timeOn(day time.Time, loc *time.Location) time.Time {
	candidate := time.Date(day.Year(), day.Month(), day.Day(), s.At.Hour, s.At.Minute, 0, 0, loc)

	wanted := s.At.Hour*60 + s.At.Minute
	got := candidate.Hour()*60 + candidate.Minute()
	if got != wanted {
		candidate = candidate.Add(time.Duration(wanted-got) * time.Minute)
	}

	return candidate
}

// fallsOn reports whether the schedule fires on the given day, ignoring the
// time of day.
func (s Schedule) fallsOn(day time.Time) bool {
	switch s.Kind {
	case KindDaily:
		return true

	case KindWeekdays:
		weekday := day.Weekday()
		return weekday != time.Saturday && weekday != time.Sunday

	case KindWeekly:
		for _, d := range s.Weekdays {
			if d == day.Weekday() {
				return true
			}
		}
		return false

	case KindMonthly:
		return day.Day() == effectiveDayOfMonth(s.DayOfMonth, day)

	case KindOnce:
		// Handled in NextRun, which never reaches here.
		return false

	default:
		return false
	}
}

// effectiveDayOfMonth clamps a day-of-month to the length of the month the
// given day belongs to, so a reminder set for the 31st fires on the last day of
// February rather than being skipped for four months of the year.
func effectiveDayOfMonth(dayOfMonth int, in time.Time) int {
	if last := daysInMonth(in); dayOfMonth > last {
		return last
	}
	return dayOfMonth
}

// daysInMonth returns the number of days in the month the given time falls in.
func daysInMonth(t time.Time) int {
	// Day 0 of the following month is the last day of this one.
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
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
