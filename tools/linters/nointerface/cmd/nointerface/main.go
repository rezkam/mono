package main

import (
	"github.com/rezkam/mono/tools/linters/nointerface"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(nointerface.Analyzer)
}
