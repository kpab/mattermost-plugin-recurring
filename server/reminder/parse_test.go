package reminder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRecurring(t *testing.T) {
	tests := []struct {
		input       string
		wantKind    Kind
		wantAt      TimeOfDay
		wantDays    []time.Weekday
		wantDayOfMo int
		wantMessage string
	}{
		// Japanese
		{
			input: "毎週月曜 10:00 週次報告", wantKind: KindWeekly,
			wantAt: TimeOfDay{Hour: 10}, wantDays: []time.Weekday{time.Monday},
			wantMessage: "週次報告",
		},
		{
			input: "毎週金曜日 18:30 週報を出す", wantKind: KindWeekly,
			wantAt: TimeOfDay{Hour: 18, Minute: 30}, wantDays: []time.Weekday{time.Friday},
			wantMessage: "週報を出す",
		},
		{
			input: "毎日 9:00 朝会", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 9}, wantMessage: "朝会",
		},
		{
			input: "毎朝9時 ストレッチ", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 9}, wantMessage: "ストレッチ",
		},
		{
			input: "毎日9時半 水を飲む", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 9, Minute: 30}, wantMessage: "水を飲む",
		},
		{
			input: "毎晩 22時30分 日記", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 22, Minute: 30}, wantMessage: "日記",
		},
		{
			input: "平日 18:00 退勤", wantKind: KindWeekdays,
			wantAt: TimeOfDay{Hour: 18}, wantMessage: "退勤",
		},
		{
			input: "毎月1日 9:00 経費精算", wantKind: KindMonthly,
			wantAt: TimeOfDay{Hour: 9}, wantDayOfMo: 1, wantMessage: "経費精算",
		},
		{
			input: "毎月25日 10:00 請求書", wantKind: KindMonthly,
			wantAt: TimeOfDay{Hour: 10}, wantDayOfMo: 25, wantMessage: "請求書",
		},
		{
			// Full-width digits and colon, as a Japanese IME produces them.
			input: "毎日　９：００　朝会", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 9}, wantMessage: "朝会",
		},

		// English
		{
			input: "every monday 10:00 weekly report", wantKind: KindWeekly,
			wantAt: TimeOfDay{Hour: 10}, wantDays: []time.Weekday{time.Monday},
			wantMessage: "weekly report",
		},
		{
			input: "every monday at 10:00 weekly report", wantKind: KindWeekly,
			wantAt: TimeOfDay{Hour: 10}, wantDays: []time.Weekday{time.Monday},
			wantMessage: "weekly report",
		},
		{
			input: "mondays 9am stand-up", wantKind: KindWeekly,
			wantAt: TimeOfDay{Hour: 9}, wantDays: []time.Weekday{time.Monday},
			wantMessage: "stand-up",
		},
		{
			input: "daily 9:00 stand-up", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 9}, wantMessage: "stand-up",
		},
		{
			input: "every day at 9am stand-up", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 9}, wantMessage: "stand-up",
		},
		{
			input: "every day at 9pm wind down", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 21}, wantMessage: "wind down",
		},
		{
			input: "weekdays 18:00 log off", wantKind: KindWeekdays,
			wantAt: TimeOfDay{Hour: 18}, wantMessage: "log off",
		},
		{
			input: "every weekday 18:00 log off", wantKind: KindWeekdays,
			wantAt: TimeOfDay{Hour: 18}, wantMessage: "log off",
		},
		{
			input: "monthly on the 1st 9:00 expenses", wantKind: KindMonthly,
			wantAt: TimeOfDay{Hour: 9}, wantDayOfMo: 1, wantMessage: "expenses",
		},
		{
			input: "every month on the 25th 10:00 invoices", wantKind: KindMonthly,
			wantAt: TimeOfDay{Hour: 10}, wantDayOfMo: 25, wantMessage: "invoices",
		},
		{
			// Case should not matter.
			input: "EVERY MONDAY 10:00 Weekly Report", wantKind: KindWeekly,
			wantAt: TimeOfDay{Hour: 10}, wantDays: []time.Weekday{time.Monday},
			wantMessage: "Weekly Report",
		},
		{
			// Midnight and noon are the meridiem edge cases.
			input: "daily 12am midnight check", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 0}, wantMessage: "midnight check",
		},
		{
			input: "daily 12pm lunch", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 12}, wantMessage: "lunch",
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			schedule, message, err := ParseRecurring(tc.input)
			require.NoError(t, err)

			assert.Equal(t, tc.wantKind, schedule.Kind)
			assert.Equal(t, tc.wantAt, schedule.At)
			assert.Equal(t, tc.wantMessage, message)

			if tc.wantDays != nil {
				assert.Equal(t, tc.wantDays, schedule.Weekdays)
			}
			if tc.wantDayOfMo != 0 {
				assert.Equal(t, tc.wantDayOfMo, schedule.DayOfMonth)
			}

			assert.NoError(t, schedule.Validate(), "parser produced a schedule that fails validation")
		})
	}
}

// "every weekday" must not be swallowed by the "every day" rule.
func TestParseRecurringPrefersWeekdaysOverDaily(t *testing.T) {
	schedule, _, err := ParseRecurring("every weekday 09:00 stand-up")
	require.NoError(t, err)
	assert.Equal(t, KindWeekdays, schedule.Kind)
}

func TestParseRecurringErrors(t *testing.T) {
	tests := map[string]struct {
		input   string
		wantErr error
	}{
		"empty":                {input: "", wantErr: ErrNoSchedule},
		"blank":                {input: "   ", wantErr: ErrNoSchedule},
		"no recurrence phrase": {input: "10:00 weekly report", wantErr: ErrNoSchedule},
		"schedule but no message": {
			input: "毎週月曜 10:00", wantErr: ErrNoMessage,
		},
		"english schedule but no message": {
			input: "every monday at 10:00", wantErr: ErrNoMessage,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := ParseRecurring(tc.input)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestParseRecurringRejectsImpossibleTimes(t *testing.T) {
	for _, input := range []string{
		"毎日 25:00 無理",
		"毎日 10:99 無理",
		"daily 25:00 nope",
		"daily 13pm nope",
		"毎日 25時 無理",
	} {
		t.Run(input, func(t *testing.T) {
			_, _, err := ParseRecurring(input)
			assert.Error(t, err)
		})
	}
}

func TestParseRecurringRejectsMissingTime(t *testing.T) {
	_, _, err := ParseRecurring("毎週月曜 週次報告")
	assert.Error(t, err)
}

func TestParseRecurringRejectsOverlongMessage(t *testing.T) {
	long := make([]byte, MaxMessageLength+1)
	for i := range long {
		long[i] = 'a'
	}

	_, _, err := ParseRecurring("daily 9:00 " + string(long))
	assert.Error(t, err)
}

func TestDescribe(t *testing.T) {
	tests := map[string]struct {
		schedule Schedule
		want     string
	}{
		"daily": {
			schedule: Schedule{Kind: KindDaily, At: TimeOfDay{Hour: 9}},
			want:     "every day at 09:00",
		},
		"weekdays": {
			schedule: Schedule{Kind: KindWeekdays, At: TimeOfDay{Hour: 18, Minute: 30}},
			want:     "every weekday at 18:30",
		},
		"weekly": {
			schedule: Schedule{Kind: KindWeekly, At: TimeOfDay{Hour: 10}, Weekdays: []time.Weekday{time.Monday}},
			want:     "every Monday at 10:00",
		},
		"weekly with several days": {
			schedule: Schedule{
				Kind: KindWeekly, At: TimeOfDay{Hour: 10},
				Weekdays: []time.Weekday{time.Monday, time.Thursday},
			},
			want: "every Monday, Thursday at 10:00",
		},
		"monthly 1st": {
			schedule: Schedule{Kind: KindMonthly, At: TimeOfDay{Hour: 9}, DayOfMonth: 1},
			want:     "on the 1st of every month at 09:00",
		},
		"monthly 22nd": {
			schedule: Schedule{Kind: KindMonthly, At: TimeOfDay{Hour: 9}, DayOfMonth: 22},
			want:     "on the 22nd of every month at 09:00",
		},
		"monthly 11th is not 11st": {
			schedule: Schedule{Kind: KindMonthly, At: TimeOfDay{Hour: 9}, DayOfMonth: 11},
			want:     "on the 11th of every month at 09:00",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.schedule.Describe())
		})
	}
}

// Anything the parser accepts must round trip into a description without
// tripping over an unhandled kind.
func TestParsedSchedulesAllDescribe(t *testing.T) {
	for _, input := range []string{
		"毎日 9:00 a",
		"平日 9:00 a",
		"毎週月曜 9:00 a",
		"毎月3日 9:00 a",
	} {
		schedule, _, err := ParseRecurring(input)
		require.NoError(t, err)
		assert.NotEqual(t, "unknown schedule", schedule.Describe())
	}
}
