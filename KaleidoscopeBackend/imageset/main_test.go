package imageset

import (
	"log"
	"os"
	"testing"
)

// TestMain computes plain white's hash before any test runs, as main does at startup.
func TestMain(m *testing.M) {

	if err := ComputePlainPageHash(); err != nil {
		log.Fatal(err)
	}
	os.Exit(m.Run())
}
