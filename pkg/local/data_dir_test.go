package local

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSetDataDir(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, previous, flag, env, want string
		wantErr                         error
	}{
		{name: "flag wins", previous: "/prev", flag: "/flag", env: "/env", want: "/flag"},
		{name: "env when no flag", previous: "/prev", env: "/env", want: "/env"},
		{name: "previous when no flag or env", previous: "/prev", want: "/prev"},
		{name: "relative path is made absolute", flag: "data", want: filepath.Join(cwd, "data")},
		// G88: an empty path used to become the current folder
		{name: "nothing given is an error, not the current folder", wantErr: ErrEmptyDataDir},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(DATA_DIR_ENV_NAME, c.env)
			prevRemembered := previousDataDir
			previousDataDir = "/remembered"
			t.Cleanup(func() { previousDataDir = prevRemembered })
			err := SetDataDir(c.previous, c.flag)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("SetDataDir error = %v, want %v", err, c.wantErr)
				}
				if got := os.Getenv(DATA_DIR_ENV_NAME); got != "" {
					t.Fatalf("%s was set to %q on error", DATA_DIR_ENV_NAME, got)
				}
				if got := GetPreviousServerDataDir(); got != "/remembered" {
					t.Fatalf("the remembered previous dir changed to %q on error", got)
				}
				return
			}
			if got := GetPreviousServerDataDir(); got != c.previous {
				t.Fatalf("previous dir = %q, want %q", got, c.previous)
			}
			if err != nil {
				t.Fatalf("SetDataDir: %v", err)
			}
			if got := os.Getenv(DATA_DIR_ENV_NAME); got != c.want {
				t.Fatalf("%s = %q, want %q", DATA_DIR_ENV_NAME, got, c.want)
			}
		})
	}
}
