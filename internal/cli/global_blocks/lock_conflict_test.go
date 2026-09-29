package global_blocks

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestIsLockConflict(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"deadlock", &mysql.MySQLError{Number: 1213}, true},
		{"lock wait timeout", &mysql.MySQLError{Number: 1205}, true},
		{"wrapped deadlock", fmt.Errorf("failed to insert rule: %w", &mysql.MySQLError{Number: 1213}), true},
		{"fk violation", &mysql.MySQLError{Number: 1452}, false},
		{"savepoint missing", &mysql.MySQLError{Number: 1305}, false},
		{"plain error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := isLockConflict(c.err); got != c.want {
			t.Errorf("%s: isLockConflict = %v, want %v", c.name, got, c.want)
		}
	}
}
