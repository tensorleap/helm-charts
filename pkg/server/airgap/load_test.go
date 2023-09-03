package airgap

import (
	"fmt"
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Skip("skip test")

	file, err := os.Open("pack.tar")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	manifest, _, _, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(manifest)

}
