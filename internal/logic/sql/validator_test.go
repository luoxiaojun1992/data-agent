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

func TestValidateCTE(t *testing.T) {
	sql := "WITH cte AS (SELECT * FROM users) SELECT * FROM cte"
	r := sqllogic.Validate(sql, nil)
	if !r.Allowed {
		t.Errorf("CTE (WITH) should be allowed, got: %s", r.Reason)
	}
}

func TestValidateUnion(t *testing.T) {
	tests := []string{
		"SELECT * FROM users UNION SELECT * FROM admins",
		"SELECT * FROM users UNION ALL SELECT * FROM admins",
	}
	for _, sql := range tests {
		r := sqllogic.Validate(sql, nil)
		if !r.Allowed {
			t.Errorf("UNION should be allowed: %s, reason: %s", sql, r.Reason)
		}
	}
}

func TestValidateShallowSubquery(t *testing.T) {
	// Shallow subqueries should be allowed (depth <= 2)
	tests := []string{
		"SELECT * FROM (SELECT * FROM users)",
		"SELECT * FROM (SELECT * FROM (SELECT * FROM users))",
	}
	for _, sql := range tests {
		r := sqllogic.Validate(sql, nil)
		if !r.Allowed {
			t.Errorf("shallow subquery should be allowed: %s, reason: %s", sql, r.Reason)
		}
	}
}

func TestValidateMultiStatement(t *testing.T) {
	// Multi-statement: if first is SELECT but contains DROP later, it should be blocked
	sql := "SELECT * FROM users; DROP TABLE users"
	r := sqllogic.Validate(sql, nil)
	if r.Allowed {
		t.Error("multi-statement with DROP should be blocked")
	}
}

func TestValidateAllowedStatements(t *testing.T) {
	tests := []string{
		"DESCRIBE users",
		"SHOW TABLES",
		"SHOW COLUMNS FROM users",
		"EXPLAIN SELECT * FROM users",
		"EXPLAIN ANALYZE SELECT * FROM users",
		"SELECT COUNT(*) FROM users",
		"SELECT DISTINCT name FROM users",
		"SELECT * FROM users ORDER BY id DESC LIMIT 10",
		"SELECT u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id",
	}
	for _, sql := range tests {
		t.Run(sql, func(t *testing.T) {
			r := sqllogic.Validate(sql, nil)
			if !r.Allowed {
				t.Errorf("statement should be allowed: %q, reason: %s", sql, r.Reason)
			}
		})
	}
}

func TestValidateBlockedStatements(t *testing.T) {
	tests := []struct {
		sql    string
		reason string
	}{
		{"DELETE FROM users WHERE id = 1", "DELETE"},
		{"DROP TABLE users", "DROP"},
		{"DROP DATABASE mydb", "DROP"},
		{"INSERT INTO users VALUES (1)", "INSERT"},
		{"INSERT INTO users (name) VALUES ('test')", "INSERT"},
		{"UPDATE users SET name = 'test' WHERE id = 1", "UPDATE"},
		{"CREATE TABLE users (id INT)", "CREATE"},
		{"ALTER TABLE users ADD COLUMN age INT", "ALTER"},
		{"TRUNCATE TABLE users", "TRUNCATE"},
		{"GRANT SELECT ON users TO user1", "GRANT"},
		{"REVOKE SELECT ON users FROM user1", "REVOKE"},
	}
	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			r := sqllogic.Validate(tt.sql, nil)
			if r.Allowed {
				t.Errorf("statement should be blocked: %q", tt.sql)
			}
		})
	}
}

func TestValidateDangerousFunctions(t *testing.T) {
	tests := []string{
		"SELECT * FROM users WHERE SLEEP(5)",
		"SELECT BENCHMARK(1000000, MD5('test'))",
		"SELECT LOAD_FILE('/etc/passwd')",
		"SELECT * FROM users INTO OUTFILE '/tmp/data'",
		"SELECT * FROM users INTO DUMPFILE '/tmp/data'",
	}
	for _, sql := range tests {
		t.Run(sql, func(t *testing.T) {
			r := sqllogic.Validate(sql, nil)
			if r.Allowed {
				t.Errorf("dangerous function should be blocked: %q", sql)
			}
		})
	}
}

func TestValidateBlockUnknownStatement(t *testing.T) {
	r := sqllogic.Validate("UNKNOWNCOMMAND users", nil)
	if r.Allowed {
		t.Error("unknown statement types should be blocked")
	}
}

func TestValidateParameterizedQuery(t *testing.T) {
	// Parameterized queries are allowed
	r := sqllogic.Validate("SELECT * FROM users WHERE name = ?", []interface{}{"test"})
	if !r.Allowed {
		t.Errorf("parameterized SELECT should be allowed, got: %s", r.Reason)
	}
}

// ── Comprehensive DESCRIBE Tests ──

func TestValidateDescribeVariants(t *testing.T) {
	tests := []string{
		"DESCRIBE users",
		"DESCRIBE SELECT * FROM users",
	}
	for _, sql := range tests {
		t.Run(sql, func(t *testing.T) {
			r := sqllogic.Validate(sql, nil)
			if !r.Allowed {
				t.Errorf("DESCRIBE should be allowed: %q, reason: %s", sql, r.Reason)
			}
		})
	}
}

// ── Comprehensive SHOW Tests ──

func TestValidateShowVariants(t *testing.T) {
	tests := []string{
		"SHOW DATABASES",
		"SHOW TABLES",
		"SHOW TABLES FROM mydb",
		"SHOW COLUMNS FROM users",
		"SHOW INDEX FROM users",
		"SHOW FULL TABLES",
		"SHOW STATUS",
		"SHOW VARIABLES",
		"SHOW WARNINGS",
	}
	for _, sql := range tests {
		t.Run(sql, func(t *testing.T) {
			r := sqllogic.Validate(sql, nil)
			if !r.Allowed {
				t.Errorf("SHOW should be allowed: %q, reason: %s", sql, r.Reason)
			}
		})
	}
}

func TestValidateShowCreateTableBlocked(t *testing.T) {
	// "SHOW CREATE TABLE" is blocked because "CREATE" is a dangerous DDL keyword
	r := sqllogic.Validate("SHOW CREATE TABLE users", nil)
	if r.Allowed {
		t.Error("SHOW CREATE TABLE should be blocked because CREATE is a DDL keyword")
	}
}

// ── Comprehensive EXPLAIN Tests ──

func TestValidateExplainVariants(t *testing.T) {
	tests := []string{
		"EXPLAIN SELECT * FROM users",
		"EXPLAIN ANALYZE SELECT * FROM users WHERE id = 1",
		"EXPLAIN FORMAT=JSON SELECT * FROM users",
	}
	for _, sql := range tests {
		t.Run(sql, func(t *testing.T) {
			r := sqllogic.Validate(sql, nil)
			if !r.Allowed {
				t.Errorf("EXPLAIN should be allowed: %q, reason: %s", sql, r.Reason)
			}
		})
	}
}

// ── Complex SELECT Tests ──

func TestValidateComplexSelect(t *testing.T) {
	tests := []string{
		"SELECT * FROM users GROUP BY department HAVING COUNT(*) > 1",
		"SELECT * FROM users ORDER BY id DESC LIMIT 10",
		"SELECT u.name FROM users u INNER JOIN orders o ON u.id = o.user_id",
		"SELECT u.name FROM users u LEFT JOIN orders o ON u.id = o.user_id",
		"SELECT * FROM users WHERE EXISTS (SELECT 1 FROM orders WHERE orders.user_id = users.id)",
		"SELECT DISTINCT department FROM users",
		"SELECT COUNT(*) as cnt, department FROM users GROUP BY department",
		"SELECT * FROM users WHERE name LIKE 'test%'",
		"SELECT * FROM users WHERE id IN (1, 2, 3)",
		"SELECT * FROM users WHERE id BETWEEN 1 AND 100",
	}
	for _, sql := range tests {
		t.Run(sql, func(t *testing.T) {
			r := sqllogic.Validate(sql, nil)
			if !r.Allowed {
				t.Errorf("complex SELECT should be allowed: %q, reason: %s", sql, r.Reason)
			}
		})
	}
}

// ── Window Function Tests ──

func TestValidateWindowFunctions(t *testing.T) {
	tests := []string{
		"SELECT ROW_NUMBER() OVER (PARTITION BY department ORDER BY salary DESC) as rn, name FROM users",
		"SELECT RANK() OVER (ORDER BY score DESC) as ranking, name FROM students",
		"SELECT SUM(amount) OVER (ORDER BY created_at) as running_total FROM orders",
	}
	for _, sql := range tests {
		t.Run(sql, func(t *testing.T) {
			r := sqllogic.Validate(sql, nil)
			if !r.Allowed {
				t.Errorf("window function SELECT should be allowed: %q, reason: %s", sql, r.Reason)
			}
		})
	}
}

// ── Subquery Depth Tests ──

func TestValidateSubqueryDepthExactly2(t *testing.T) {
	// Depth 2 — should be allowed
	sql := "SELECT * FROM (SELECT * FROM (SELECT * FROM users))"
	r := sqllogic.Validate(sql, nil)
	if !r.Allowed {
		t.Errorf("subquery depth 2 should be allowed, got: %s", r.Reason)
	}
}

func TestValidateSubqueryDepthExactly3(t *testing.T) {
	// Depth 3 — should be blocked
	sql := "SELECT * FROM (SELECT * FROM (SELECT * FROM (SELECT * FROM users)))"
	r := sqllogic.Validate(sql, nil)
	if r.Allowed {
		t.Error("subquery depth 3 should be blocked")
	}
}

func TestValidateSubqueryDepthNoParens(t *testing.T) {
	// SELECT without parentheses should not increase depth
	sql := "SELECT * FROM users WHERE id IN (SELECT id FROM orders)"
	r := sqllogic.Validate(sql, nil)
	if !r.Allowed {
		t.Errorf("simple subquery without parens should be allowed, got: %s", r.Reason)
	}
}

// ── Edge Cases ──

func TestValidateEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		allowed bool
	}{
		{"empty string", "", false},
		{"whitespace only", "   \t\n  ", false},
		{"only newlines", "\n\n\n", false},
		{"unicode in string", "SELECT * FROM users WHERE name = '日本語'", true},
		{"unicode table alias", "SELECT * FROM users AS 用户", true},
		{"trailing semicolon", "SELECT * FROM users;", true},
		{"multiple whitespace", "SELECT   *   FROM    users", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := sqllogic.Validate(tt.sql, nil)
			if r.Allowed != tt.allowed {
				t.Errorf("Validate(%q) allowed=%v, want=%v, reason=%s", tt.sql, r.Allowed, tt.allowed, r.Reason)
			}
		})
	}
}

func TestValidateExtremelyLongSQL(t *testing.T) {
	// Build a very long but valid SELECT
	longSQL := "SELECT " + repeatString("col000,", 1000) + " * FROM users"
	r := sqllogic.Validate(longSQL, nil)
	if !r.Allowed {
		t.Errorf("long valid SQL should be allowed, got: %s", r.Reason)
	}
}

func repeatString(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// ── Each Dangerous Function Independently ──

func TestValidateDangerousSleep(t *testing.T) {
	r := sqllogic.Validate("SELECT * FROM users WHERE SLEEP(5)", nil)
	if r.Allowed {
		t.Error("SLEEP should be blocked")
	}
}

func TestValidateDangerousBenchmark(t *testing.T) {
	r := sqllogic.Validate("SELECT BENCHMARK(1000000, MD5('test'))", nil)
	if r.Allowed {
		t.Error("BENCHMARK should be blocked")
	}
}

func TestValidateDangerousLoadFile(t *testing.T) {
	r := sqllogic.Validate("SELECT LOAD_FILE('/etc/passwd')", nil)
	if r.Allowed {
		t.Error("LOAD_FILE should be blocked")
	}
}

func TestValidateDangerousIntoOutfile(t *testing.T) {
	r := sqllogic.Validate("SELECT * FROM users INTO OUTFILE '/tmp/data'", nil)
	if r.Allowed {
		t.Error("INTO OUTFILE should be blocked")
	}
}

func TestValidateDangerousIntoDumpfile(t *testing.T) {
	r := sqllogic.Validate("SELECT * FROM users INTO DUMPFILE '/tmp/data'", nil)
	if r.Allowed {
		t.Error("INTO DUMPFILE should be blocked")
	}
}

// ── All DDL Blocking Tests ──

func TestValidateBlockCreateTable(t *testing.T) {
	r := sqllogic.Validate("CREATE TABLE users (id INT PRIMARY KEY)", nil)
	if r.Allowed {
		t.Error("CREATE TABLE should be blocked")
	}
}

func TestValidateBlockCreateIndex(t *testing.T) {
	r := sqllogic.Validate("CREATE INDEX idx_name ON users (name)", nil)
	if r.Allowed {
		t.Error("CREATE INDEX should be blocked")
	}
}

func TestValidateBlockCreateView(t *testing.T) {
	r := sqllogic.Validate("CREATE VIEW user_view AS SELECT * FROM users", nil)
	if r.Allowed {
		t.Error("CREATE VIEW should be blocked")
	}
}

func TestValidateBlockCreateDatabase(t *testing.T) {
	r := sqllogic.Validate("CREATE DATABASE mydb", nil)
	if r.Allowed {
		t.Error("CREATE DATABASE should be blocked")
	}
}

func TestValidateBlockAlterTable(t *testing.T) {
	r := sqllogic.Validate("ALTER TABLE users ADD COLUMN age INT", nil)
	if r.Allowed {
		t.Error("ALTER TABLE should be blocked")
	}
}

func TestValidateBlockTruncate(t *testing.T) {
	r := sqllogic.Validate("TRUNCATE TABLE users", nil)
	if r.Allowed {
		t.Error("TRUNCATE should be blocked")
	}
}

func TestValidateBlockUpdateNested(t *testing.T) {
	// UPDATE somewhere in the middle
	r := sqllogic.Validate("SELECT * FROM users WHERE UPDATE users SET name='x'", nil)
	if r.Allowed {
		t.Error("embedded UPDATE should be blocked")
	}
}

// ── Parameterized Queries ──

func TestValidateParameterizedPositionalDollar(t *testing.T) {
	r := sqllogic.Validate("SELECT * FROM users WHERE id = $1 AND name = $2", []interface{}{1, "test"})
	if !r.Allowed {
		t.Errorf("parameterized SELECT with $1 should be allowed, got: %s", r.Reason)
	}
}

func TestValidateParameterizedMultipleQMark(t *testing.T) {
	r := sqllogic.Validate("SELECT * FROM users WHERE id = ? AND status = ?", []interface{}{1, "active"})
	if !r.Allowed {
		t.Errorf("multi-param SELECT should be allowed, got: %s", r.Reason)
	}
}

// ── SafeFields Tests ──

func TestSafeFields(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		want   []string
	}{
		{"normal", []string{"id", "name", "email"}, []string{"id", "name", "email"}},
		{"sql injection", []string{"id; DROP TABLE users", "1=1"}, []string{"idDROPTABLEusers", "11"}},
		{"mixed valid/invalid", []string{"valid_col", "bad-col"}, []string{"valid_col", "badcol"}},
		{"spaces", []string{"  id  "}, []string{"id"}},
		{"empty slice", []string{}, nil},
		{"nil slice", nil, nil},
		{"unicode", []string{"用户名"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sqllogic.SafeFields(tt.fields)
			if len(got) != len(tt.want) {
				t.Errorf("SafeFields len=%d, want=%d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("SafeFields[%d]=%q, want=%q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ── Mixed Case Tests ──

func TestValidateMixedCase(t *testing.T) {
	tests := []string{
		"select * from users",
		"Select * From Users",
		"SELECT * FROM users",
		"SeLeCt * FrOm users",
	}
	for _, sql := range tests {
		t.Run(sql, func(t *testing.T) {
			r := sqllogic.Validate(sql, nil)
			if !r.Allowed {
				t.Errorf("mixed case should be allowed: %q, reason: %s", sql, r.Reason)
			}
		})
	}
}

// ── Comment-Containing SQL ──

func TestValidateWithComments(t *testing.T) {
	tests := []string{
		"SELECT * FROM users -- this is a comment",
		"SELECT * FROM /* comment */ users",
	}
	for _, sql := range tests {
		t.Run(sql, func(t *testing.T) {
			r := sqllogic.Validate(sql, nil)
			if !r.Allowed {
				t.Errorf("SQL with comments should be allowed: %q, reason: %s", sql, r.Reason)
			}
		})
	}
}

// ── SELECT INTO ──

func TestValidateSelectInto(t *testing.T) {
	// SELECT INTO variable is different from INTO OUTFILE
	sql := "SELECT COUNT(*) INTO @cnt FROM users"
	r := sqllogic.Validate(sql, nil)
	if !r.Allowed {
		t.Errorf("SELECT INTO variable should be allowed, got: %s", r.Reason)
	}
}
