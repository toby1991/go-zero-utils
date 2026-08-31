// Package timex 提供时间戳解析与上海时区日界计算等共享时间工具。
//
// 以下历史契约刻意保留，请勿"修复"：
// 时间戳 1e10 的秒/毫秒阈值、上海日界返回昨日/今日/明日的顺序、
// 相邻日期在 UTC 上固定 AddDate（不修正历史夏令时）等。
package timex

import "time"

// tzShanghai 为 Asia/Shanghai 时区，包初始化时加载，加载失败直接 panic。
// 不对外暴露：调用方不应依赖此 Location 的内部状态。
var tzShanghai *time.Location

func init() {
	var err error
	tzShanghai, err = time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
}

// ParseTimestamp 将时间戳解析为 time.Time。
// 时间戳大于 1e10 时按毫秒（13 位）处理，否则按秒（10 位）处理；
// 该阈值为历史契约，请勿修改。返回值保留 time.Unix / time.UnixMilli 的
// 本地时区 Location，需要 UTC 时请调用 .UTC()。
func ParseTimestamp(ts int64) time.Time {
	if ts > 1e10 {
		return time.UnixMilli(ts)
	} else {
		return time.Unix(ts, 0)
	}
}

// TodayAtShanghaiInUTC 返回当前时刻对应的上海自然日界，
// 顺序为 昨日开始、今日开始、明日开始，均为 UTC 时刻。
// 语义等价于 TimeAtShanghaiInUTC(time.Now())；若调用前后恰好跨越
// 上海午夜，结果可能随之变化，对稳定性敏感的场景请改用 TimeAtShanghaiInUTC。
func TodayAtShanghaiInUTC() (yesterday, today, tomorrow time.Time) {
	return TimeAtShanghaiInUTC(time.Now())
}

// TimeAtShanghaiInUTC 以 timeAtUTC 对应的上海日期计算昨日、今日、明日的日界，
// 返回顺序为 昨日开始、今日开始、明日开始，均为 UTC 且零纳秒。
//
// 今日开始为上海当地 0 点对应的 UTC 时刻；昨日、明日由今日在 UTC 上
// AddDate(0, 0, ±1) 固定加减 24 小时得到，不修正历史夏令时：
// 例如 1991 年上海实行夏令时的日期，相邻日界可能与真实上海午夜相差 1 小时。
// 此行为为历史契约，刻意保留。
//
// timeAtUTC 的 Location 不限（内部会转换到上海时区判断日期），
// 但零值 time.Time 会被视为真实时刻（公元 1 年 1 月 1 日），不作空值哨兵。
func TimeAtShanghaiInUTC(timeAtUTC time.Time) (yesterday, today, tomorrow time.Time) {
	timeAtShanghai := timeAtUTC.In(tzShanghai)

	todayStartedAtShanghai := time.Date(timeAtShanghai.Year(), timeAtShanghai.Month(), timeAtShanghai.Day(), 0, 0, 0, 0, tzShanghai)

	today = todayStartedAtShanghai.UTC()
	tomorrow = today.AddDate(0, 0, 1)
	yesterday = today.AddDate(0, 0, -1)

	return yesterday, today, tomorrow
}

// ThisMinuteInUTC 返回 timeAtUTC 所在分钟的半开区间 [startedAt, endedAt)：
// startedAt 为 timeAtUTC 截断到分钟的时刻，endedAt 恰为 startedAt 加一分钟。
//
// 不主动转换时区，直接对入参做绝对时间截断，调用方应传入 UTC 时刻。
// time.Time 零值被视为真实时刻，不作空值哨兵。
func ThisMinuteInUTC(timeAtUTC time.Time) (startedAt, endedAt time.Time) {
	startedAt = timeAtUTC.Truncate(time.Minute)
	endedAt = startedAt.Add(time.Minute)

	return startedAt, endedAt
}
