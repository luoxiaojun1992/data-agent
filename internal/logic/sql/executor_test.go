package sql

import (
	"strings"
	"testing"
)

// Execute short-circuits on an unsafe statement *before* opening any DB
// connection, so the rejection path is testable without a live MySQL.
func TestExecute_RejectsDDL(t *testing.T) {
	res, err := Execute(ExecConfig{}, "DROP TABLE users", nil)
	if err == nil {
		t.Fatal("expected rejection error for DDL")
	}
	if res.Status != "rejected" {
		t.Errorf("status = %q, want rejected", res.Status)
	}
	if res.Query != "DROP TABLE users" {
		t.Errorf("query = %q", res.Query)
	}
	if res.Message == "" || !strings.Contains(res.Message, "DROP") {
		t.Errorf("message = %q, want to mention the rejected keyword", res.Message)
	}
	if !strings.Contains(err.Error(), "rejected") {
		t.Errorf("err = %v, want to mention rejected", err)
	}
}

func TestExecute_RejectsNonReadStatement(t *testing.T) {
	// "DELETE" hits the dangerous-keyword list; a bare non-read prefix hits the
	// allow-list check. Both must fail-closed without a DB.
	for _, q := range []string{"DELETE FROM users", "INSERT INTO t VALUES (1)", "VACUUM"} {
		res, err := Execute(ExecConfig{}, q, nil)
		if err == nil {
			t.Errorf("Execute(%q): expected rejection, got nil error", q)
		}
		if res.Status != "rejected" {
			t.Errorf("Execute(%q): status = %q, want rejected", q, res.Status)
		}
	}
}
