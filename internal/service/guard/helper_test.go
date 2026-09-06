package guard

import "testing"

func TestParseIntent_TaskNoPlan(t *testing.T) {
	r := parseIntent(`{"is_task": true, "is_plan": false}`)
	if !r.IsTask {
		t.Errorf("is_task = %v, want true", r.IsTask)
	}
	if r.IsPlan {
		t.Errorf("is_plan = %v, want false", r.IsPlan)
	}
}

func TestParseIntent_PlanTask(t *testing.T) {
	r := parseIntent(`{"is_task": true, "is_plan": true}`)
	if !r.IsTask {
		t.Errorf("is_task = %v, want true", r.IsTask)
	}
	if !r.IsPlan {
		t.Errorf("is_plan = %v, want true", r.IsPlan)
	}
}

func TestParseIntent_Chat(t *testing.T) {
	r := parseIntent(`{"is_task": false, "is_plan": false}`)
	if r.IsTask {
		t.Errorf("is_task = %v, want false", r.IsTask)
	}
	if r.IsPlan {
		t.Errorf("is_plan = %v, want false", r.IsPlan)
	}
}

func TestParseIntent_PlanImpliedByTaskOnly(t *testing.T) {
	// is_plan 缺省（旧模型只回 is_task）时默认 false，不误判为需要规划。
	r := parseIntent(`{"is_task": true}`)
	if !r.IsTask {
		t.Errorf("is_task = %v, want true", r.IsTask)
	}
	if r.IsPlan {
		t.Errorf("is_plan = %v, want false when omitted", r.IsPlan)
	}
}

func TestParseIntent_MalformedDefaultsToChat(t *testing.T) {
	for _, s := range []string{"", "not json", "```json\n{\"is_task\": true}\n``` broken"} {
		r := parseIntent(s)
		if r.IsTask || r.IsPlan {
			t.Errorf("parseIntent(%q) = %+v, want chat default", s, r)
		}
	}
}

func TestParseIntent_CodeFence(t *testing.T) {
	r := parseIntent("```json\n{\"is_task\": true, \"is_plan\": true}\n```")
	if !r.IsTask || !r.IsPlan {
		t.Errorf("code-fenced parse = %+v, want task+plan", r)
	}
}
