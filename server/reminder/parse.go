package reminder

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Parsing is deliberately hand written rather than handed to a date library:
// the ones available for Go are built around English prose and cannot read
// "毎週月曜 10:00", which is the phrasing the first users of this plugin will
// reach for. Keeping it here also keeps the dependency list short, which
// matters for getting through a Marketplace security review.

// Each parse error names what was missing and shows something that works.
// A reminder that will not parse is the most common way to meet this plugin,
// so the error is the documentation most people read.
var (
	// ErrNoSchedule is returned when the input does not begin with something
	// that looks like a schedule.
	ErrNoSchedule = errors.New(
		"couldn't work out how often to repeat this. Put the repeat first, then the time, then the message — like `every monday at 10:00 weekly report`")

	// ErrNoTime is returned when the repeat was understood but no time of day
	// followed it.
	ErrNoTime = errors.New(
		"the repeat is clear, but not the time. Add one like `10:00`, `9am` or `18:30`")

	// ErrNoMessage is returned when a schedule was understood but nothing was
	// left to remind the user about.
	ErrNoMessage = errors.New(
		"that's when, but not what. Put the message after the time — like `every day at 09:00 stand-up`")
)

// fullWidth maps the characters a Japanese IME produces to their ASCII
// equivalents, so "１０：００" parses the same as "10:00".
var fullWidth = strings.NewReplacer(
	"０", "0", "１", "1", "２", "2", "３", "3", "４", "4",
	"５", "5", "６", "6", "７", "7", "８", "8", "９", "9",
	"：", ":", "　", " ",
)

var japaneseWeekdays = map[string]time.Weekday{
	"日": time.Sunday,
	"月": time.Monday,
	"火": time.Tuesday,
	"水": time.Wednesday,
	"木": time.Thursday,
	"金": time.Friday,
	"土": time.Saturday,
}

var englishWeekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "sun": time.Sunday,
	"monday": time.Monday, "mon": time.Monday,
	"tuesday": time.Tuesday, "tue": time.Tuesday, "tues": time.Tuesday,
	"wednesday": time.Wednesday, "wed": time.Wednesday,
	"thursday": time.Thursday, "thu": time.Thursday, "thurs": time.Thursday,
	"friday": time.Friday, "fri": time.Friday,
	"saturday": time.Saturday, "sat": time.Saturday,
}

// English patterns all end in \b. Without it "monthly" starts with "mon" and
// gets read as Monday, and "daily" would shadow longer phrases the same way.
// The Japanese patterns cannot use \b — there are no word boundaries between
// Japanese characters — so the two languages are kept apart.
//
// Case insensitivity is expressed with (?i) rather than by lower-casing the
// input first: strings.ToLower does not preserve byte length (U+212A KELVIN
// SIGN is three bytes and lower-cases to a one-byte "k"), so an offset taken
// from the lower-cased string would slice the original in the wrong place.
var (
	reDailyJP = regexp.MustCompile(`^(?:毎日|毎朝|毎晩|毎夜)\s*`)
	reDailyEN = regexp.MustCompile(`(?i)^(?:every\s+day|daily)\b\s*`)

	reWeekdaysJP = regexp.MustCompile(`^(?:平日|毎平日)\s*`)
	reWeekdaysEN = regexp.MustCompile(`(?i)^(?:every\s+weekdays?|on\s+weekdays?|weekdays?)\b\s*`)

	reWeeklyJP = regexp.MustCompile(`^毎週\s*([日月火水木金土])曜(?:日)?\s*`)
	reWeeklyEN = regexp.MustCompile(`(?i)^(?:every\s+)?(sunday|sun|monday|mon|tuesday|tues|tue|wednesday|wed|thursday|thurs|thu|friday|fri|saturday|sat)s?\b\s*`)

	reMonthlyJP = regexp.MustCompile(`^毎月\s*(\d{1,2})\s*日\s*`)
	reMonthlyEN = regexp.MustCompile(`(?i)^(?:every\s+month\s+on\s+(?:the\s+)?|monthly\s+on\s+(?:the\s+)?)(\d{1,2})(?:st|nd|rd|th)?\b\s*`)

	// Times, tried in order. The 24-hour form is first so "18:30" is not read
	// as a bare hour. It also swallows a trailing am/pm: without that, "9:00am"
	// leaves "am" at the head of the message, and "9:00pm" silently means 09:00.
	reTimeColon    = regexp.MustCompile(`(?i)^(\d{1,2}):(\d{2})\s*(am|pm)?\s*`)
	reTimeJPHalf   = regexp.MustCompile(`^(\d{1,2})\s*時\s*半\s*`)
	reTimeJPMinute = regexp.MustCompile(`^(\d{1,2})\s*時\s*(\d{1,2})\s*分?\s*`)
	reTimeJPHour   = regexp.MustCompile(`^(\d{1,2})\s*時\s*`)
	reTimeMeridiem = regexp.MustCompile(`(?i)^(?:at\s+)?(\d{1,2})\s*(am|pm)\s*`)

	// Optional filler between the schedule and the time, e.g. "every day at 9am".
	reAt = regexp.MustCompile(`(?i)^(?:at\b|の)\s*`)

	// A bare hour, only accepted after an explicit "at" — otherwise the leading
	// number of a message would be eaten as a time.
	reTimeBareHour = regexp.MustCompile(`^(\d{1,2})\b\s*`)
)

// ParseRecurring reads a recurring schedule and its message out of one line of
// user input, e.g. "毎週月曜 10:00 週次報告" or "every monday at 10:00 weekly report".
//
// One-off reminders ("in 30 minutes") are not handled here yet; they arrive in M2.
func ParseRecurring(input string) (Schedule, string, error) {
	rest := strings.TrimSpace(fullWidth.Replace(input))
	if rest == "" {
		return Schedule{}, "", ErrNoSchedule
	}

	schedule, rest, err := parseRecurrence(rest)
	if err != nil {
		return Schedule{}, "", err
	}

	rest = strings.TrimSpace(rest)
	afterAt := reAt.MatchString(rest)
	rest = reAt.ReplaceAllString(rest, "")

	at, rest, err := parseTimeOfDay(rest, afterAt)
	if err != nil {
		return Schedule{}, "", err
	}
	schedule.At = at

	message := strings.TrimSpace(rest)
	if message == "" {
		return Schedule{}, "", ErrNoMessage
	}
	if utf8.RuneCountInString(message) > MaxMessageLength {
		return Schedule{}, "", fmt.Errorf("that message is too long — keep it under %d characters", MaxMessageLength)
	}

	if err := schedule.Validate(); err != nil {
		return Schedule{}, "", err
	}

	return schedule, message, nil
}

// parseRecurrence consumes the leading recurrence phrase.
func parseRecurrence(input string) (Schedule, string, error) {
	// Weekdays before daily: "every weekday" must not be eaten by "every day".
	if m := reWeekdaysJP.FindString(input); m != "" {
		return Schedule{Kind: KindWeekdays}, input[len(m):], nil
	}

	if m := reWeekdaysEN.FindString(input); m != "" {
		return Schedule{Kind: KindWeekdays}, input[len(m):], nil
	}

	if m := reDailyJP.FindString(input); m != "" {
		return Schedule{Kind: KindDaily}, input[len(m):], nil
	}

	if m := reDailyEN.FindString(input); m != "" {
		return Schedule{Kind: KindDaily}, input[len(m):], nil
	}

	if m := reWeeklyJP.FindStringSubmatch(input); m != nil {
		return Schedule{
			Kind:     KindWeekly,
			Weekdays: []time.Weekday{japaneseWeekdays[m[1]]},
		}, input[len(m[0]):], nil
	}

	if m := reWeeklyEN.FindStringSubmatch(input); m != nil {
		return Schedule{
			Kind:     KindWeekly,
			Weekdays: []time.Weekday{englishWeekdays[strings.ToLower(m[1])]},
		}, input[len(m[0]):], nil
	}

	if m := reMonthlyJP.FindStringSubmatch(input); m != nil {
		day, err := strconv.Atoi(m[1])
		if err != nil {
			return Schedule{}, "", ErrNoSchedule
		}
		return Schedule{Kind: KindMonthly, DayOfMonth: day}, input[len(m[0]):], nil
	}

	if m := reMonthlyEN.FindStringSubmatch(input); m != nil {
		day, err := strconv.Atoi(m[1])
		if err != nil {
			return Schedule{}, "", ErrNoSchedule
		}
		return Schedule{Kind: KindMonthly, DayOfMonth: day}, input[len(m[0]):], nil
	}

	return Schedule{}, "", ErrNoSchedule
}

// parseTimeOfDay consumes the leading time of day. afterAt reports whether an
// explicit "at" preceded it, which is what makes a bare hour ("at 9") safe to
// accept without eating the first number of a message.
func parseTimeOfDay(input string, afterAt bool) (TimeOfDay, string, error) {
	if m := reTimeColon.FindStringSubmatch(input); m != nil {
		hour, minute := atoi(m[1]), atoi(m[2])
		if meridiem := strings.ToLower(m[3]); meridiem != "" {
			converted, err := applyMeridiem(hour, meridiem)
			if err != nil {
				return TimeOfDay{}, "", err
			}
			hour = converted
		} else if hour > 23 {
			return TimeOfDay{}, "", invalidTime(m[0])
		}
		if minute > 59 {
			return TimeOfDay{}, "", invalidTime(m[0])
		}
		return TimeOfDay{Hour: hour, Minute: minute}, input[len(m[0]):], nil
	}

	if m := reTimeJPHalf.FindStringSubmatch(input); m != nil {
		hour := atoi(m[1])
		if hour > 23 {
			return TimeOfDay{}, "", invalidTime(m[0])
		}
		return TimeOfDay{Hour: hour, Minute: 30}, input[len(m[0]):], nil
	}

	if m := reTimeJPMinute.FindStringSubmatch(input); m != nil {
		hour, minute := atoi(m[1]), atoi(m[2])
		if hour > 23 || minute > 59 {
			return TimeOfDay{}, "", invalidTime(m[0])
		}
		return TimeOfDay{Hour: hour, Minute: minute}, input[len(m[0]):], nil
	}

	if m := reTimeJPHour.FindStringSubmatch(input); m != nil {
		hour := atoi(m[1])
		if hour > 23 {
			return TimeOfDay{}, "", invalidTime(m[0])
		}
		return TimeOfDay{Hour: hour}, input[len(m[0]):], nil
	}

	if m := reTimeMeridiem.FindStringSubmatch(input); m != nil {
		hour, err := applyMeridiem(atoi(m[1]), strings.ToLower(m[2]))
		if err != nil {
			return TimeOfDay{}, "", err
		}
		return TimeOfDay{Hour: hour}, input[len(m[0]):], nil
	}

	if afterAt {
		if m := reTimeBareHour.FindStringSubmatch(input); m != nil {
			hour := atoi(m[1])
			if hour > 23 {
				return TimeOfDay{}, "", invalidTime(m[0])
			}
			return TimeOfDay{Hour: hour}, input[len(m[0]):], nil
		}
	}

	return TimeOfDay{}, "", ErrNoTime
}

// invalidTime reports a time that parsed but is out of range.
func invalidTime(matched string) error {
	return fmt.Errorf("`%s` isn't a valid time — hours run 0 to 23, minutes 0 to 59", strings.TrimSpace(matched))
}

// applyMeridiem converts a 12-hour clock reading to 24-hour.
func applyMeridiem(hour int, meridiem string) (int, error) {
	if hour < 1 || hour > 12 {
		return 0, fmt.Errorf("`%d%s` isn't a valid time — with am or pm the hour runs 1 to 12", hour, meridiem)
	}

	switch {
	case meridiem == "pm" && hour != 12:
		return hour + 12, nil
	case meridiem == "am" && hour == 12:
		return 0, nil
	default:
		return hour, nil
	}
}

// atoi parses digits already matched by a regexp, so it cannot fail.
func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// Describe renders the schedule back into English, for confirming what was
// understood and for listing reminders.
func (s Schedule) Describe() string {
	at := fmt.Sprintf("%02d:%02d", s.At.Hour, s.At.Minute)

	switch s.Kind {
	case KindOnce:
		return "once at " + time.UnixMilli(s.OnceAt).UTC().Format("2006-01-02 15:04 MST")
	case KindDaily:
		return "every day at " + at
	case KindWeekdays:
		return "every weekday at " + at
	case KindWeekly:
		days := make([]string, 0, len(s.Weekdays))
		for _, d := range s.Weekdays {
			days = append(days, d.String())
		}
		return "every " + strings.Join(days, ", ") + " at " + at
	case KindMonthly:
		return fmt.Sprintf("on the %s of every month at %s", ordinal(s.DayOfMonth), at)
	default:
		return "unknown schedule"
	}
}

// ordinal renders 1 as "1st", 2 as "2nd", and so on.
func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}

	return strconv.Itoa(n) + suffix
}
