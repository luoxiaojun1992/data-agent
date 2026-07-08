package sql_test

import (
	"testing"

	sqllogic "github.com/luoxiaojun1992/data-agent/internal/logic/sql"
)

func TestValidateAllowSelect(t *testing.T) {
	r := sqllogic.Validate("SELECT * FROM users WHERE id = ?", []interface{}{1})
	if !r.Allowed {
		t.Errorf("SELECT should be allowed, got: %s", r.Reason)
	}
}

func TestValidateBlockDrop(t *testing.T) {
	r := sqllogic.Validate("DROP TABLE users", nil)
	if r.Allowed {
		t.Error("DROP TABLE should be blocked")
	}
}

func TestValidateBlockDelete(t *testing.T) {
	r := sqllogic.Validate("DELETE FROM users WHERE id = 1", nil)
	if r.Allowed {
		t.Error("DELETE should be blocked")
	}
}

func TestValidateBlockInsert(t *testing.T) {
	r := sqllogic.Validate("INSERT INTO users VALUES (1)", nil)
	if r.Allowed {
		t.Error("INSERT should be blocked")
	}
}

func TestValidateBlockDangerousFunctions(t *testing.T) {
	r := sqllogic.Validate("SELECT * FROM users WHERE SLEEP(5)", nil)
	if r.Allowed {
		t.Error("SLEEP function should be blocked")
	}
}

func TestValidateSubqueryDepth(t *testing.T) {
	sql := "SELECT * FROM (SELECT * FROM (SELECT * FROM (SELECT * FROM t)))"
	r := sqllogic.Validate(sql, nil)
	if r.Allowed {
		t.Error("Deep subqueries should be blocked")
	}
}
