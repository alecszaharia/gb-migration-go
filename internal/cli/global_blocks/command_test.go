package global_blocks

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestWorkersOption(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     string
		want    int
		wantErr bool
	}{
		{name: "long flag", args: []string{"--workers", "3"}, want: 3},
		{name: "short flag", args: []string{"-w", "3"}, want: 3},
		{name: "env var", env: "3", want: 3},
		{name: "flag overrides env", args: []string{"-w", "5"}, env: "3", want: 5},
		{name: "default", want: 4},
		{name: "zero flag", args: []string{"--workers", "0"}, wantErr: true},
		{name: "negative flag", args: []string{"-w", "-1"}, wantErr: true},
		{name: "zero env", env: "0", wantErr: true},
		{name: "negative env", env: "-2", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			if tt.env != "" {
				t.Setenv("WORKERS", tt.env)
			}

			cmd := NewCommand("migrations")
			ran := false
			got := 0
			// Stub RunE so no database is ever touched.
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				ran = true
				got = viper.GetInt("workers")
				return nil
			}
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true

			err := cmd.Execute()

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "invalid worker count") {
					t.Errorf("unexpected error message: %v", err)
				}
				if ran {
					t.Errorf("RunE must not run when worker count is rejected")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !ran {
				t.Fatalf("RunE was not invoked")
			}
			if got != tt.want {
				t.Errorf("workers = %d, want %d", got, tt.want)
			}
		})
	}
}
