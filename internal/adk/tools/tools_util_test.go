package adktools

import (
	"strings"
	"testing"
	"time"
)

func TestCurrentTime_FieldsAndTimezone(t *testing.T) {
	// 固定 UTC 时刻：06:21:24 UTC（SPEC-100 D7 后工具按 UTC 返回）。
	now := time.Date(2026, 9, 6, 6, 21, 24, 0, time.UTC)
	got := currentTime(now)

	if got.Timezone != "UTC" {
		t.Errorf("timezone = %q, want UTC", got.Timezone)
	}
	if got.Date != "2026-09-06" {
		t.Errorf("date = %q, want 2026-09-06", got.Date)
	}
	if got.Time != "2026-09-06T06:21:24Z" {
		t.Errorf("time = %q, want 2026-09-06T06:21:24Z", got.Time)
	}
	if got.Unix != now.Unix() {
		t.Errorf("unix = %d, want %d", got.Unix, now.Unix())
	}
	// 2026-09-06 是星期日。
	if got.Weekday != "星期日" {
		t.Errorf("weekday = %q, want 星期日", got.Weekday)
	}
}

func TestCurrentTime_WeekdayMapping(t *testing.T) {
	cases := map[time.Weekday]string{
		time.Monday:    "星期一",
		time.Tuesday:   "星期二",
		time.Wednesday: "星期三",
		time.Thursday:  "星期四",
		time.Friday:    "星期五",
		time.Saturday:  "星期六",
		time.Sunday:    "星期日",
	}
	for d, want := range cases {
		if got := weekdayCN(d); got != want {
			t.Errorf("weekdayCN(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestCurrentTime_TimezoneNeverDrifts(t *testing.T) {
	// 无论服务器本地时区如何，工具结果都必须是 UTC。用两个跨 UTC 午夜的
	// 时刻验证输出始终带 Z 且日期随 UTC 翻转。
	beforeMidnight := time.Date(2026, 1, 1, 23, 0, 0, 0, time.UTC)
	afterMidnight := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	got1 := currentTime(beforeMidnight)
	got2 := currentTime(afterMidnight)

	if !strings.HasSuffix(got1.Time, "Z") || !strings.HasSuffix(got2.Time, "Z") {
		t.Errorf("expected UTC (Z) suffix, got %q and %q", got1.Time, got2.Time)
	}
	if got1.Date == got2.Date {
		t.Errorf("date should roll over at UTC midnight, got %q == %q", got1.Date, got2.Date)
	}
}

func TestGetCurrentTimeTool(t *testing.T) {
	fn := getCurrentTime()
	res, err := fn(newToolContext("time-tool"), CurrentTimeArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Time == "" || res.Date == "" || res.Weekday == "" {
		t.Errorf("result fields empty: %+v", res)
	}
	if res.Timezone != "UTC" {
		t.Errorf("timezone = %q, want UTC", res.Timezone)
	}
	if res.Unix <= 0 {
		t.Errorf("unix = %d, want > 0", res.Unix)
	}
}

func TestGetPlanMethodTool(t *testing.T) {
	fn := getPlanMethod()
	res, err := fn(newToolContext("plan-tool"), PlanMethodArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Guide != planMethodGuide {
		t.Errorf("guide not the canonical constant")
	}
	for _, want := range []string{"明确目标", "任务拆解", "排定顺序", "设定检查点", "逐步执行", "汇总交付"} {
		if !strings.Contains(res.Guide, want) {
			t.Errorf("guide missing step %q", want)
		}
	}
}
