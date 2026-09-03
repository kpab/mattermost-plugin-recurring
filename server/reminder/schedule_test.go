package reminder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()

	loc, err := time.LoadLocation(name)
	require.NoError(t, err)

	return loc
}

func TestNextRun(t *testing.T) {
	tokyo := mustLoad(t, "Asia/Tokyo")
	newYork := mustLoad(t, "America/New_York")

	at := func(hour, minute int) TimeOfDay {
		return TimeOfDay{Hour: hour, Minute: minute}
	}

	tests := []struct {
		name     string
		schedule Schedule
		loc      *time.Location
		after    time.Time
		want     time.Time
		wantOK   bool
	}{
		{
			name:     "daily later the same day",
			schedule: Schedule{Kind: KindDaily, At: at(9, 0)},
			loc:      tokyo,
			after:    time.Date(2026, time.September, 3, 7, 30, 0, 0, tokyo),
			want:     time.Date(2026, time.September, 3, 9, 0, 0, 0, tokyo),
			wantOK:   true,
		},
		{
			name:     "daily rolls over to tomorrow once today's time has passed",
			schedule: Schedule{Kind: KindDaily, At: at(9, 0)},
			loc:      tokyo,
			after:    time.Date(2026, time.September, 3, 9, 30, 0, 0, tokyo),
			want:     time.Date(2026, time.September, 4, 9, 0, 0, 0, tokyo),
			wantOK:   true,
		},
		{
			name: "the firing time itself is not a valid next run",
			// Guards the scheduler's re-arm path: computing the next run from
			// the moment a reminder just fired must not return that same moment.
			schedule: Schedule{Kind: KindDaily, At: at(9, 0)},
			loc:      tokyo,
			after:    time.Date(2026, time.September, 3, 9, 0, 0, 0, tokyo),
			want:     time.Date(2026, time.September, 4, 9, 0, 0, 0, tokyo),
			wantOK:   true,
		},
		{
			name:     "weekdays skip the weekend",
			schedule: Schedule{Kind: KindWeekdays, At: at(18, 30)},
			loc:      tokyo,
			// 2026-09-04 is a Friday.
			after:  time.Date(2026, time.September, 4, 19, 0, 0, 0, tokyo),
			want:   time.Date(2026, time.September, 7, 18, 30, 0, 0, tokyo),
			wantOK: true,
		},
		{
			name:     "weekly wraps to next week",
			schedule: Schedule{Kind: KindWeekly, At: at(10, 0), Weekdays: []time.Weekday{time.Monday}},
			loc:      tokyo,
			// 2026-09-08 is a Tuesday, so the next Monday is a week out.
			after:  time.Date(2026, time.September, 8, 0, 0, 0, 0, tokyo),
			want:   time.Date(2026, time.September, 14, 10, 0, 0, 0, tokyo),
			wantOK: true,
		},
		{
			name: "weekly picks the nearest of several weekdays",
			schedule: Schedule{
				Kind:     KindWeekly,
				At:       at(10, 0),
				Weekdays: []time.Weekday{time.Monday, time.Thursday},
			},
			loc: tokyo,
			// Tuesday: Thursday comes before next Monday.
			after:  time.Date(2026, time.September, 8, 0, 0, 0, 0, tokyo),
			want:   time.Date(2026, time.September, 10, 10, 0, 0, 0, tokyo),
			wantOK: true,
		},
		{
			name:     "monthly on a day the month has",
			schedule: Schedule{Kind: KindMonthly, At: at(9, 0), DayOfMonth: 15},
			loc:      tokyo,
			after:    time.Date(2026, time.September, 3, 0, 0, 0, 0, tokyo),
			want:     time.Date(2026, time.September, 15, 9, 0, 0, 0, tokyo),
			wantOK:   true,
		},
		{
			name:     "monthly on the 31st clamps to the end of a short month",
			schedule: Schedule{Kind: KindMonthly, At: at(9, 0), DayOfMonth: 31},
			loc:      tokyo,
			after:    time.Date(2026, time.February, 1, 0, 0, 0, 0, tokyo),
			want:     time.Date(2026, time.February, 28, 9, 0, 0, 0, tokyo),
			wantOK:   true,
		},
		{
			name:     "monthly on the 31st reaches Feb 29 in a leap year",
			schedule: Schedule{Kind: KindMonthly, At: at(9, 0), DayOfMonth: 31},
			loc:      tokyo,
			after:    time.Date(2028, time.February, 1, 0, 0, 0, 0, tokyo),
			want:     time.Date(2028, time.February, 29, 9, 0, 0, 0, tokyo),
			wantOK:   true,
		},
		{
			name:     "monthly on the 30th skips nothing in a 31 day month",
			schedule: Schedule{Kind: KindMonthly, At: at(9, 0), DayOfMonth: 30},
			loc:      tokyo,
			after:    time.Date(2026, time.January, 30, 9, 0, 0, 0, tokyo),
			want:     time.Date(2026, time.February, 28, 9, 0, 0, 0, tokyo),
			wantOK:   true,
		},
		{
			name:     "daily keeps its wall-clock time across the spring DST jump",
			schedule: Schedule{Kind: KindDaily, At: at(9, 0)},
			loc:      newYork,
			// 2026-03-08 is when US clocks jump from 02:00 to 03:00.
			after:  time.Date(2026, time.March, 7, 12, 0, 0, 0, newYork),
			want:   time.Date(2026, time.March, 8, 9, 0, 0, 0, newYork),
			wantOK: true,
		},
		{
			name:     "daily keeps its wall-clock time across the autumn DST fallback",
			schedule: Schedule{Kind: KindDaily, At: at(9, 0)},
			loc:      newYork,
			// 2026-11-01 is when US clocks fall back from 02:00 to 01:00.
			after:  time.Date(2026, time.October, 31, 12, 0, 0, 0, newYork),
			want:   time.Date(2026, time.November, 1, 9, 0, 0, 0, newYork),
			wantOK: true,
		},
		{
			name:     "one-off in the future",
			schedule: Schedule{Kind: KindOnce, OnceAt: time.Date(2026, time.September, 3, 9, 0, 0, 0, tokyo).UnixMilli()},
			loc:      tokyo,
			after:    time.Date(2026, time.September, 3, 8, 0, 0, 0, tokyo),
			want:     time.Date(2026, time.September, 3, 9, 0, 0, 0, tokyo),
			wantOK:   true,
		},
		{
			name:     "one-off already past never fires again",
			schedule: Schedule{Kind: KindOnce, OnceAt: time.Date(2026, time.September, 3, 9, 0, 0, 0, tokyo).UnixMilli()},
			loc:      tokyo,
			after:    time.Date(2026, time.September, 3, 9, 0, 0, 1, tokyo),
			wantOK:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.schedule.NextRun(tc.after, tc.loc)

			require.Equal(t, tc.wantOK, ok)
			if !tc.wantOK {
				return
			}

			assert.True(t, got.Equal(tc.want),
				"got %s, want %s", got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
		})
	}
}

// A daily reminder set for a wall-clock time that a DST jump erases still has
// to fire that day. Go normalises the missing time forward, which is the
// behaviour we want; this test pins it down so a future refactor cannot
// silently drop the day instead.
func TestNextRunDuringDSTGap(t *testing.T) {
	newYork := mustLoad(t, "America/New_York")

	schedule := Schedule{Kind: KindDaily, At: TimeOfDay{Hour: 2, Minute: 30}}
	after := time.Date(2026, time.March, 7, 12, 0, 0, 0, newYork)

	got, ok := schedule.NextRun(after, newYork)
	require.True(t, ok)

	assert.Equal(t, 2026, got.Year())
	assert.Equal(t, time.March, got.Month())
	assert.Equal(t, 8, got.Day(), "the reminder must still fire on the day of the jump")
	assert.Equal(t, 3, got.Hour(), "02:30 does not exist that day, so it lands at 03:30")
	assert.Equal(t, 30, got.Minute())
}

func TestNextRunRepeatsForward(t *testing.T) {
	tokyo := mustLoad(t, "Asia/Tokyo")

	schedule := Schedule{Kind: KindWeekly, At: TimeOfDay{Hour: 10}, Weekdays: []time.Weekday{time.Monday}}
	cursor := time.Date(2026, time.September, 1, 0, 0, 0, 0, tokyo)

	var fired []string
	for range 3 {
		next, ok := schedule.NextRun(cursor, tokyo)
		require.True(t, ok)
		fired = append(fired, next.Format("2006-01-02 15:04"))
		cursor = next
	}

	// Feeding each firing time back in must march forward a week at a time.
	assert.Equal(t, []string{
		"2026-09-07 10:00",
		"2026-09-14 10:00",
		"2026-09-21 10:00",
	}, fired)
}

func TestNextRunDefaultsToUTC(t *testing.T) {
	schedule := Schedule{Kind: KindDaily, At: TimeOfDay{Hour: 9}}
	after := time.Date(2026, time.September, 3, 7, 0, 0, 0, time.UTC)

	got, ok := schedule.NextRun(after, nil)
	require.True(t, ok)
	assert.True(t, got.Equal(time.Date(2026, time.September, 3, 9, 0, 0, 0, time.UTC)))
}

func TestReminderAdvance(t *testing.T) {
	tokyo := mustLoad(t, "Asia/Tokyo")
	now := time.Date(2026, time.September, 3, 9, 30, 0, 0, tokyo)

	t.Run("recurring reminder is rescheduled", func(t *testing.T) {
		r := &Reminder{
			Schedule: Schedule{Kind: KindDaily, At: TimeOfDay{Hour: 9}},
			Timezone: "Asia/Tokyo",
		}

		require.True(t, r.Advance(now))
		assert.Equal(t, time.Date(2026, time.September, 4, 9, 0, 0, 0, tokyo).UnixMilli(), r.NextRunAt)
	})

	t.Run("spent one-off reminder is cleared", func(t *testing.T) {
		r := &Reminder{
			Schedule:  Schedule{Kind: KindOnce, OnceAt: now.Add(-time.Hour).UnixMilli()},
			Timezone:  "Asia/Tokyo",
			NextRunAt: now.Add(-time.Hour).UnixMilli(),
		}

		require.False(t, r.Advance(now))
		assert.Zero(t, r.NextRunAt, "a reminder that will not fire again must not stay queued")
	})
}

func TestLocationFallsBackToUTC(t *testing.T) {
	r := &Reminder{Timezone: "Mars/Olympus"}

	assert.Equal(t, time.UTC, r.Location())
}

// A daily reminder must fire once a day, every day, in every timezone. The
// original implementation stepped a timestamp rather than a calendar date and
// so skipped the day entirely in zones whose clocks jump at midnight
// (America/Santiago each September), which a weekly schedule turns into a
// skipped week and a monthly one into a skipped month.
func TestNextRunNeverSkipsADay(t *testing.T) {
	zones := []string{
		"Asia/Tokyo",       // no DST
		"America/New_York", // springs forward at 02:00
		"America/Santiago", // springs forward at 00:00
		"America/Havana",   // springs forward at 00:00
		"America/Asuncion", // springs forward at 00:00
		"Australia/Lord_Howe",
		"Pacific/Chatham",
		"Europe/Lisbon",
	}

	for _, name := range zones {
		t.Run(name, func(t *testing.T) {
			loc := mustLoad(t, name)
			schedule := Schedule{Kind: KindDaily, At: TimeOfDay{Hour: 9}}

			cursor := time.Date(2020, time.January, 1, 0, 0, 0, 0, loc)
			end := time.Date(2030, time.January, 1, 0, 0, 0, 0, loc)

			for cursor.Before(end) {
				next, ok := schedule.NextRun(cursor, loc)
				require.True(t, ok, "schedule stopped firing at %s", cursor)

				gap := next.Sub(cursor)
				assert.LessOrEqual(t, gap, 30*time.Hour,
					"gap of %s between %s and %s — a day was skipped",
					gap, cursor.Format(time.RFC3339), next.Format(time.RFC3339))

				cursor = next
			}
		})
	}
}

// A monthly reminder must fire every month, including across a midnight DST
// jump landing on the chosen day.
func TestNextRunMonthlyNeverSkipsAMonth(t *testing.T) {
	santiago := mustLoad(t, "America/Santiago")

	for day := 1; day <= 28; day++ {
		schedule := Schedule{Kind: KindMonthly, At: TimeOfDay{Hour: 9}, DayOfMonth: day}

		cursor := time.Date(2020, time.January, 1, 0, 0, 0, 0, santiago)
		end := time.Date(2030, time.January, 1, 0, 0, 0, 0, santiago)

		for cursor.Before(end) {
			next, ok := schedule.NextRun(cursor, santiago)
			require.True(t, ok)

			assert.LessOrEqual(t, next.Sub(cursor), 32*24*time.Hour,
				"day %d: gap between %s and %s spans more than a month",
				day, cursor.Format(time.RFC3339), next.Format(time.RFC3339))

			cursor = next
		}
	}
}

// Whatever a zone does to a non-existent wall-clock time, the reminder must
// never fire earlier than asked.
func TestNextRunNeverFiresEarlierThanRequested(t *testing.T) {
	zones := []string{"America/New_York", "America/Santiago", "America/Havana", "Australia/Lord_Howe"}

	for _, name := range zones {
		t.Run(name, func(t *testing.T) {
			loc := mustLoad(t, name)

			for hour := range 24 {
				for _, minute := range []int{0, 30} {
					schedule := Schedule{Kind: KindDaily, At: TimeOfDay{Hour: hour, Minute: minute}}

					cursor := time.Date(2020, time.January, 1, 0, 0, 0, 0, loc)
					end := time.Date(2030, time.January, 1, 0, 0, 0, 0, loc)

					for cursor.Before(end) {
						next, ok := schedule.NextRun(cursor, loc)
						require.True(t, ok)

						wanted := hour*60 + minute
						got := next.Hour()*60 + next.Minute()
						assert.GreaterOrEqual(t, got, wanted,
							"%02d:%02d became %s — fired earlier than requested",
							hour, minute, next.Format(time.RFC3339))

						cursor = next
					}
				}
			}
		})
	}
}
