package nointerface_test

import (
	"testing"

	"github.com/rezkam/mono/tools/linters/nointerface"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, nointerface.Analyzer, "a")
}
