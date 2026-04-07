//go:build ignore

package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
)

func main() {
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "cmd/mothership/main.go", nil, parser.AllErrors|parser.ParseComments)
	if err != nil {
		fmt.Println("Parse error:", err)
		os.Exit(1)
	}
	fmt.Println("Parse OK")
}
