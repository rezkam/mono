package main

import (
	"github.com/rezkam/mono/tools/linters/timeutc"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(timeutc.Analyzer)
}
