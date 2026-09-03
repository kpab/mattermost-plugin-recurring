// Package reminder holds the domain types for reminders and the logic that
// decides when a reminder fires next. It deliberately has no dependency on the
// Mattermost plugin API so that it stays straightforward to test.
package reminder

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Kind describes how a reminder repeats.
type Kind string

const (
	// KindOnce fires a single time at Reminder.At and is then done.
	KindOnce Kind = "once"
	// KindDaily fires every day at the configured time of day.
	KindDaily Kind = "daily"
	// KindWeekdays fires Monday through Friday at the configured time of day.
	KindWeekdays Kind = "weekdays"
	// KindWeekly fires on the configured weekdays at the configured time of day.
	KindWeekly Kind = "weekly"
	// KindMonthly fires on the configured day of the month at the configured
	// time of day. Days past the end of a short month are clamped to the last
	// day of that month, so 31 fires on Feb 28 (or 29 in a leap year).
	KindMonthly Kind = "monthly"
)

const (
	// MaxMessageLength bounds the reminder text so a single user cannot store
	// an unbounded amount of data in the KV store. Counted in runes, not bytes:
	// a Japanese message is three bytes per character and would otherwise be
	// cut off at a third of the advertised length.
	MaxMessageLength = 1000

	// MaxRemindersPerUser caps how many reminders one user can hold. Every
	// reminder is re-read and re-written on each change, and scanned on every
	// scheduler tick, so an unbounded list is a denial-of-service vector.
	MaxRemindersPerUser = 100
)

// ErrNotFound is returned when a reminder does not exist.
var ErrNotFound = errors.New("reminder not found")

// ErrTooManyReminders is returned when a user is already at MaxRemindersPerUser.
var ErrTooManyReminders = fmt.Errorf("you already have %d reminders, which is the maximum", MaxRemindersPerUser)

// TimeOfDay is a wall-clock time, interpreted in the owning user's timezone.
type TimeOfDay struct {
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
}

// Schedule describes when a reminder fires. The zero value is not valid; use
// Validate before persisting one.
type Schedule struct {
	Kind Kind      `json:"kind"`
	At   TimeOfDay `json:"at"`

	// Weekdays is used by KindWeekly. Values are time.Weekday, Sunday is 0.
	Weekdays []time.Weekday `json:"weekdays,omitempty"`

	// DayOfMonth is used by KindMonthly, 1-31.
	DayOfMonth int `json:"day_of_month,omitempty"`

	// OnceAt is used by KindOnce. Unix milliseconds, UTC.
	OnceAt int64 `json:"once_at,omitempty"`
}

// Validate reports whether the schedule is well formed.
func (s Schedule) Validate() error {
	if s.Kind != KindOnce {
		if s.At.Hour < 0 || s.At.Hour > 23 {
			return fmt.Errorf("hour must be between 0 and 23, got %d", s.At.Hour)
		}
		if s.At.Minute < 0 || s.At.Minute > 59 {
			return fmt.Errorf("minute must be between 0 and 59, got %d", s.At.Minute)
		}
	}

	switch s.Kind {
	case KindOnce:
		if s.OnceAt <= 0 {
			return errors.New("a one-off reminder needs a time to fire at")
		}
	case KindDaily, KindWeekdays:
		// Nothing beyond the time of day.
	case KindWeekly:
		if len(s.Weekdays) == 0 {
			return errors.New("a weekly reminder needs at least one weekday")
		}
		seen := map[time.Weekday]bool{}
		for _, d := range s.Weekdays {
			if d < time.Sunday || d > time.Saturday {
				return fmt.Errorf("invalid weekday %d", d)
			}
			if seen[d] {
				return fmt.Errorf("weekday %s is listed more than once", d)
			}
			seen[d] = true
		}
	case KindMonthly:
		if s.DayOfMonth < 1 || s.DayOfMonth > 31 {
			return fmt.Errorf("day of month must be between 1 and 31, got %d", s.DayOfMonth)
		}
	default:
		return fmt.Errorf("unknown schedule kind %q", s.Kind)
	}

	return nil
}

// Recurring reports whether the schedule fires more than once.
func (s Schedule) Recurring() bool {
	return s.Kind != KindOnce
}

// Reminder is a single reminder owned by one user.
type Reminder struct {
	ID      string `json:"id"`
	UserID  string `json:"user_id"`
	Message string `json:"message"`

	Schedule Schedule `json:"schedule"`

	// Timezone is the IANA name the schedule is interpreted in, captured when
	// the reminder is created so that a later change to the user's timezone
	// does not silently move every existing reminder.
	Timezone string `json:"timezone"`

	// NextRunAt is Unix milliseconds in UTC. Zero means the reminder is not
	// scheduled to fire again.
	NextRunAt int64 `json:"next_run_at"`

	// Completed marks a reminder the user has ticked off. A completed
	// recurring reminder stops firing but is kept until deleted.
	Completed bool `json:"completed"`

	// FailureCount counts consecutive delivery failures. A reminder addressed
	// to someone who can no longer receive it would otherwise be retried
	// forever, so delivery gives up once this passes MaxDeliveryFailures.
	FailureCount int `json:"failure_count,omitempty"`

	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// MaxDeliveryFailures is how many consecutive delivery failures a reminder
// survives before it is parked.
const MaxDeliveryFailures = 10

// Validate reports whether the reminder is well formed.
func (r *Reminder) Validate() error {
	if r.ID == "" {
		return errors.New("reminder needs an id")
	}
	if r.UserID == "" {
		return errors.New("reminder needs a user id")
	}
	if strings.TrimSpace(r.Message) == "" {
		return errors.New("reminder needs a message")
	}
	if count := utf8.RuneCountInString(r.Message); count > MaxMessageLength {
		return fmt.Errorf("message must be at most %d characters, got %d", MaxMessageLength, count)
	}
	if r.Timezone == "" {
		return errors.New("reminder needs a timezone")
	}
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return fmt.Errorf("unknown timezone %q: %w", r.Timezone, err)
	}

	return r.Schedule.Validate()
}

// Due reports whether the reminder should be delivered now.
func (r *Reminder) Due(now time.Time) bool {
	return !r.Completed && r.NextRunAt > 0 && r.NextRunAt <= now.UnixMilli()
}

// Location resolves the reminder's timezone, falling back to UTC when the name
// is not known to the running system.
func (r *Reminder) Location() *time.Location {
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// Clone returns a deep copy, so callers can mutate a reminder without
// disturbing the one held by the store.
func (r *Reminder) Clone() *Reminder {
	if r == nil {
		return nil
	}
	clone := *r
	if r.Schedule.Weekdays != nil {
		clone.Schedule.Weekdays = append([]time.Weekday(nil), r.Schedule.Weekdays...)
	}
	return &clone
}
