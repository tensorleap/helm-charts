package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetReleasesPage(t *testing.T) {
	releases, err := GetReleasesPage("tensorleap", "helm-charts", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) == 0 {
		t.Fatal("no releases found")
	}
	assert.Equal(t, 10, len(releases))
}
