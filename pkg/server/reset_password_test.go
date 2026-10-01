package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShellQuote(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", "abc", "'abc'"},
		{"single quote", "p'w", `'p'\''w'`},
		{"shell metachars", "a $b `c`", "'a $b `c`'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shellQuote(tt.in))
		})
	}
}

func TestResetPasswordScript(t *testing.T) {
	script := resetPasswordScript("a@x.io", "p'w")
	for _, want := range []string{
		"-r tensorleap -q email='a@x.io' -q exact=true",
		`--new-password 'p'\''w' --temporary`,
		`create users/"$ID"/logout`,
	} {
		t.Run(want, func(t *testing.T) {
			require.Contains(t, script, want)
		})
	}
}

func TestRandomPassword(t *testing.T) {
	a, err := randomPassword(tempPasswordLength)
	require.NoError(t, err)
	b, err := randomPassword(tempPasswordLength)
	require.NoError(t, err)
	require.Len(t, a, tempPasswordLength)
	require.NotEqual(t, a, b)
	require.Empty(t, strings.Trim(a, "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"))
}
