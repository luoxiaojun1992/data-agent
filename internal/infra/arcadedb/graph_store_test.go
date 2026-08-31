package arcadedb

import (
	"errors"
	"testing"
)

func TestIsAlreadyExistsErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"generic", errors.New("connection refused"), false},
		{"neo4j constraint", errors.New("Neo.ClientError.Schema.ConstraintAlreadyExists"), true},
		{"arcadedb index", errors.New("index already exists"), true},
		{"already existing", errors.New("equivalent schema rule already existing"), true},
		{"unrelated", errors.New("syntax error at line 1"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAlreadyExistsErr(tc.err); got != tc.want {
				t.Errorf("isAlreadyExistsErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
