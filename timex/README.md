# timex

共享时间工具包，由既有业务公共库迁移而来，行为与迁移前完全一致。仅依赖标准库。

## API

| 函数 | 说明 |
| --- | --- |
| `ParseTimestamp(ts int64) time.Time` | 时间戳解析：`> 1e10` 按毫秒（13 位），否则按秒（10 位）。 |
| `TodayAtShanghaiInUTC() (yesterday, today, tomorrow time.Time)` | 当前时刻对应的上海自然日界。 |
| `TimeAtShanghaiInUTC(timeAtUTC time.Time) (yesterday, today, tomorrow time.Time)` | 任意时刻对应的上海自然日界。 |
| `ThisMinuteInUTC(timeAtUTC time.Time) (startedAt, endedAt time.Time)` | 所在分钟区间 `[startedAt, endedAt)`。 |

## 契约与边界

以下行为均为历史契约，刻意保留，请勿"修复"：

1. **秒/毫秒阈值**：`ParseTimestamp` 以 `ts > 1e10` 判定毫秒；`1e10` 本身（2286 年）走秒分支。返回值为 `time.Unix` / `time.UnixMilli` 的本地时区 Location。
2. **返回顺序**：日界函数固定返回 **昨日、今日、明日** 三个开始时刻，均为 UTC、零纳秒。今日 = 上海当地 0 点对应的 UTC 时刻。
3. **时区依赖**：包初始化时 `time.LoadLocation("Asia/Shanghai")`，加载失败直接 panic。该 Location 不对外暴露，且 `time.Time` 的时区换算依赖 IANA tzdata（含历史条目）。
4. **相邻日期不修正 DST**：昨日、明日由今日在 UTC 上 `AddDate(0, 0, ±1)` 固定加减 24 小时。历史上上海实行夏令时（如 1991 年）时，相邻日界可能与真实上海午夜相差 1 小时，此行为由测试锁定。
5. **`ThisMinuteInUTC` 不转换时区**：直接对入参做绝对时间截断并保留其 Location；调用方应传入 UTC 时刻。`endedAt` 恰为 `startedAt + 1 分钟`。
6. **零值是真实时刻**：`time.Time{}`（公元 1 年 1 月 1 日）会被正常计算，不作空值哨兵。

## 用法

```go
import "github.com/toby1991/go-zero-utils/timex"

t := timex.ParseTimestamp(1700000000000) // 毫秒 → 2023-11-14T22:13:20Z（本地 Location）

yesterday, today, tomorrow := timex.TimeAtShanghaiInUTC(time.Now())
// 例：上海日期为 2024-06-16 时 → 今日 = 2024-06-15T16:00:00Z

startedAt, endedAt := timex.ThisMinuteInUTC(time.Now().UTC())
// 例：18:34:56 → [18:34:00, 18:35:00)
```

## 测试

```sh
go test ./timex -count=1
go test -race ./timex -count=1
go vet ./timex
```
