package timex

import (
	"testing"
	"time"
)

// TestParseTimestamp 表驱动覆盖秒/毫秒解析及 1e10 阈值边界。
// 期望值使用 UTC 字面量独立构造，与实现分支无关。
func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name string
		ts   int64
		want time.Time
	}{
		{"秒-零值", 0, time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"秒-普通", 1700000000, time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC)},
		{"秒-负值", -1, time.Date(1969, 12, 31, 23, 59, 59, 0, time.UTC)},
		// 阈值本身不满足 > 1e10，必须走秒分支（2286 年），不能走毫秒分支（1970 年）。
		{"秒-阈值1e10本身", 10000000000, time.Date(2286, 11, 20, 17, 46, 40, 0, time.UTC)},
		// 阈值加一必须走毫秒分支。
		{"毫秒-阈值1e10加一", 10000000001, time.Date(1970, 4, 26, 17, 46, 40, 1e6, time.UTC)},
		// 与 1700000000 秒为同一时刻，但走毫秒分支，锁定分支判定而非巧合相等。
		{"毫秒-普通", 1700000000000, time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTimestamp(tt.ts)
			if !got.Equal(tt.want) {
				t.Fatalf("ParseTimestamp(%d) = %v，期望时刻 %v", tt.ts, got, tt.want)
			}
			// 行为契约：返回值保留 time.Unix / time.UnixMilli 的本地时区 Location。
			if got.Location() != time.Local {
				t.Fatalf("ParseTimestamp(%d) Location = %v，期望本地时区", tt.ts, got.Location())
			}
		})
	}
}

// TestTimeAtShanghaiInUTC 表驱动覆盖：上海午夜前后、UTC 晚间跨日、
// 非 UTC Location 输入，以及 1991 年上海夏令时日期锁定“UTC 固定加减一天”的历史行为。
// 期望值均为 UTC 字面量，同时统一校验输出为 UTC 且零纳秒。
func TestTimeAtShanghaiInUTC(t *testing.T) {
	utc := time.UTC
	est := time.FixedZone("EST", -5*3600)

	tests := []struct {
		name          string
		input         time.Time
		wantYesterday time.Time
		wantToday     time.Time
		wantTomorrow  time.Time
	}{
		{
			name:          "上海午夜前一刻",
			input:         time.Date(2024, 6, 15, 15, 59, 59, 0, utc), // 上海 2024-06-15 23:59:59
			wantYesterday: time.Date(2024, 6, 13, 16, 0, 0, 0, utc),
			wantToday:     time.Date(2024, 6, 14, 16, 0, 0, 0, utc),
			wantTomorrow:  time.Date(2024, 6, 15, 16, 0, 0, 0, utc),
		},
		{
			name:          "上海午夜整点-归新一日",
			input:         time.Date(2024, 6, 15, 16, 0, 0, 0, utc), // 上海 2024-06-16 00:00:00
			wantYesterday: time.Date(2024, 6, 14, 16, 0, 0, 0, utc),
			wantToday:     time.Date(2024, 6, 15, 16, 0, 0, 0, utc),
			wantTomorrow:  time.Date(2024, 6, 16, 16, 0, 0, 0, utc),
		},
		{
			name:          "UTC晚间跨日-上海已是次日",
			input:         time.Date(2024, 6, 15, 18, 30, 0, 0, utc), // 上海 2024-06-16 02:30:00
			wantYesterday: time.Date(2024, 6, 14, 16, 0, 0, 0, utc),
			wantToday:     time.Date(2024, 6, 15, 16, 0, 0, 0, utc),
			wantTomorrow:  time.Date(2024, 6, 16, 16, 0, 0, 0, utc),
		},
		{
			name:          "非UTCLocation输入-美东固定时区",
			input:         time.Date(2024, 3, 10, 5, 30, 0, 0, est), // = 2024-03-10 10:30:00Z，上海 18:30
			wantYesterday: time.Date(2024, 3, 8, 16, 0, 0, 0, utc),
			wantToday:     time.Date(2024, 3, 9, 16, 0, 0, 0, utc),
			wantTomorrow:  time.Date(2024, 3, 10, 16, 0, 0, 0, utc),
		},
		{
			// 锁定历史 DST 行为：1991-04-14 起上海实行夏令时（+09）。
			// 今日（04-14）为真实上海午夜 1991-04-13T16:00Z（+08），
			// 但“明日”由今日在 UTC 固定 AddDate(+1) 得 1991-04-14T16:00Z，
			// 而非真实上海午夜 1991-04-14T15:00Z（+09）——该 1 小时偏差为兼容契约，刻意保留。
			name:          "1991上海夏令时-锁定UTC固定加减一天",
			input:         time.Date(1991, 4, 14, 0, 0, 0, 0, utc), // 上海 1991-04-14 08:00:00（+08，夏令时 02:00 才开始）
			wantYesterday: time.Date(1991, 4, 12, 16, 0, 0, 0, utc),
			wantToday:     time.Date(1991, 4, 13, 16, 0, 0, 0, utc),
			wantTomorrow:  time.Date(1991, 4, 14, 16, 0, 0, 0, utc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yesterday, today, tomorrow := TimeAtShanghaiInUTC(tt.input)

			if !yesterday.Equal(tt.wantYesterday) || !today.Equal(tt.wantToday) || !tomorrow.Equal(tt.wantTomorrow) {
				t.Fatalf("TimeAtShanghaiInUTC(%v)\n  昨日 = %v（期望 %v）\n  今日 = %v（期望 %v）\n  明日 = %v（期望 %v）",
					tt.input, yesterday, tt.wantYesterday, today, tt.wantToday, tomorrow, tt.wantTomorrow)
			}

			// 输出契约：均为 UTC、零纳秒。
			for name, got := range map[string]time.Time{"昨日": yesterday, "今日": today, "明日": tomorrow} {
				if got.Location() != time.UTC {
					t.Fatalf("%s Location = %v，期望 UTC", name, got.Location())
				}
				if got.Nanosecond() != 0 {
					t.Fatalf("%s Nanosecond = %d，期望 0", name, got.Nanosecond())
				}
			}

			// 相邻日界在 UTC 上固定间隔 24 小时（历史 DST 不修正的直接体现）。
			if today.Sub(yesterday) != 24*time.Hour || tomorrow.Sub(today) != 24*time.Hour {
				t.Fatalf("相邻日界间隔 = %v / %v，期望均为 24 小时", today.Sub(yesterday), tomorrow.Sub(today))
			}
		})
	}
}

// TestTimeAtShanghaiInUTC_ZeroValue 验证零值 time.Time 被当作真实时刻处理：
// 公元 1 年 1 月 1 日对应的上海日界，而非空值哨兵。
// 该年份上海时区偏移受 tzdata 中 LMT 条目影响，期望值通过 tzShanghai 构造，避免硬编码脆弱的字面值。
func TestTimeAtShanghaiInUTC_ZeroValue(t *testing.T) {
	yesterday, today, tomorrow := TimeAtShanghaiInUTC(time.Time{})

	wantToday := time.Date(1, 1, 1, 0, 0, 0, 0, tzShanghai).UTC()
	if !today.Equal(wantToday) {
		t.Fatalf("今日 = %v，期望 %v", today, wantToday)
	}
	if !yesterday.Equal(wantToday.AddDate(0, 0, -1)) || !tomorrow.Equal(wantToday.AddDate(0, 0, 1)) {
		t.Fatalf("昨日 = %v，明日 = %v，期望为今日固定 ±1 天", yesterday, tomorrow)
	}

	if today.IsZero() {
		t.Fatal("零值输入的今日输出仍是零值时刻，疑似被当作空值哨兵处理")
	}
	if today.Location() != time.UTC || today.Nanosecond() != 0 {
		t.Fatalf("今日 = %v，期望 UTC 且零纳秒", today)
	}

	// 今日转换回上海时区必须是当地 0 点整，验证日界语义的往返一致性。
	y, m, d := today.In(tzShanghai).Date()
	hour, min, sec := today.In(tzShanghai).Clock()
	if hour != 0 || min != 0 || sec != 0 || today.In(tzShanghai).Nanosecond() != 0 {
		t.Fatalf("今日回读上海 = %v-%v-%v %v:%v:%v，期望当地 0 点整", y, m, d, hour, min, sec)
	}
}

// TestTodayAtShanghaiInUTC 用调用前后两个候选时刻断言，避免偶发跨越上海午夜导致 flaky。
func TestTodayAtShanghaiInUTC(t *testing.T) {
	before := time.Now()
	yesterday, today, tomorrow := TodayAtShanghaiInUTC()
	after := time.Now()

	y0, t0, tm0 := TimeAtShanghaiInUTC(before)
	y1, t1, tm1 := TimeAtShanghaiInUTC(after)

	match0 := yesterday.Equal(y0) && today.Equal(t0) && tomorrow.Equal(tm0)
	match1 := yesterday.Equal(y1) && today.Equal(t1) && tomorrow.Equal(tm1)
	if !match0 && !match1 {
		t.Fatalf("今日 = %v，与调用前后候选 %v / %v 均不一致", today, t0, t1)
	}

	for name, got := range map[string]time.Time{"昨日": yesterday, "今日": today, "明日": tomorrow} {
		if got.Location() != time.UTC || got.Nanosecond() != 0 {
			t.Fatalf("%s = %v，期望 UTC 且零纳秒", name, got)
		}
	}
}

// TestThisMinuteInUTC 表驱动覆盖分钟截断及结束时间恰好增加一分钟。
func TestThisMinuteInUTC(t *testing.T) {
	utc := time.UTC

	tests := []struct {
		name      string
		input     time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "秒与纳秒被截断",
			input:     time.Date(2024, 6, 15, 18, 34, 56, 789000000, utc),
			wantStart: time.Date(2024, 6, 15, 18, 34, 0, 0, utc),
			wantEnd:   time.Date(2024, 6, 15, 18, 35, 0, 0, utc),
		},
		{
			name:      "整分输入保持不变",
			input:     time.Date(2024, 6, 15, 18, 34, 0, 0, utc),
			wantStart: time.Date(2024, 6, 15, 18, 34, 0, 0, utc),
			wantEnd:   time.Date(2024, 6, 15, 18, 35, 0, 0, utc),
		},
		{
			name:      "零值输入视为真实时刻",
			input:     time.Time{},
			wantStart: time.Time{},
			wantEnd:   time.Date(1, 1, 1, 0, 1, 0, 0, utc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startedAt, endedAt := ThisMinuteInUTC(tt.input)

			if !startedAt.Equal(tt.wantStart) || !endedAt.Equal(tt.wantEnd) {
				t.Fatalf("ThisMinuteInUTC(%v) = [%v, %v)，期望 [%v, %v)",
					tt.input, startedAt, endedAt, tt.wantStart, tt.wantEnd)
			}
			if startedAt.Second() != 0 || startedAt.Nanosecond() != 0 {
				t.Fatalf("startedAt = %v，期望秒与纳秒均为 0", startedAt)
			}
			if !endedAt.Equal(startedAt.Add(time.Minute)) {
				t.Fatalf("endedAt = %v，期望恰为 startedAt 加一分钟", endedAt)
			}
		})
	}
}

// TestThisMinuteInUTC_LocationPreserved 锁定“不主动转换时区”的契约：
// 入参带非 UTC Location 时结果保留原时区，仅做绝对时间截断。
func TestThisMinuteInUTC_LocationPreserved(t *testing.T) {
	cst := time.FixedZone("UTC+8", 8*3600)
	input := time.Date(2024, 6, 15, 18, 34, 56, 0, cst)

	startedAt, endedAt := ThisMinuteInUTC(input)

	if startedAt.Location() != cst || endedAt.Location() != cst {
		t.Fatalf("Location 被转换：startedAt = %v，endedAt = %v", startedAt.Location(), endedAt.Location())
	}
	if !startedAt.Equal(time.Date(2024, 6, 15, 18, 34, 0, 0, cst)) {
		t.Fatalf("startedAt = %v，期望 18:34:00 +0800", startedAt)
	}
}
