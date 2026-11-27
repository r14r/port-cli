package main

import (
	"fmt"
	"os"
)

func (o options) detailf(format string, args ...any) {
	if !o.details { return }
	w := o.progress
	if w == nil { w = os.Stdout }
	fmt.Fprintf(w, format, args...)
}

func progressStatus(failed int) string {
	if failed > 0 { return "WARN" }
	return "OK"
}
