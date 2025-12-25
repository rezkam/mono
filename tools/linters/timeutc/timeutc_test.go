package timeutc_test

import (
	"testing"

	"github.com/rezkam/mono/tools/linters/timeutc"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, timeutc.Analyzer, "a")
}
