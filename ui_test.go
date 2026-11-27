package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestUIOptions(t *testing.T) {
	o,err := parseOptions([]string{"--ui"})
	if err != nil || !o.ui || o.yes { t.Fatalf("%+v %v",o,err) }
	for _,args := range [][]string{{"--ui","--yes"},{"--ui","--dry-run"},{"--ui","--only","jq"},{"--ui","--exclude","jq"}} {
		if _,err := parseOptions(args); err == nil { t.Fatalf("accepted %v",args) }
	}
}

func TestPickerSelection(t *testing.T) {
	p := newPicker([]*row{
		{Brew:"jq",Status:"READY"},
		{Brew:"cargo-c",Status:"REVIEW"},
		{Brew:"cargo-binstall",Status:"KEEP"},
		{Brew:"lib",Status:"BLOCKED"},
		{Brew:"app",Status:"CASK"},
	},2)
	p.toggle()
	p.move(1)
	p.toggle()
	p.move(1)
	p.toggle()
	if p.offset!=1 { t.Fatalf("offset=%d",p.offset) }
	want := []string{"cargo-c","jq"}
	if got:=p.selection(); !reflect.DeepEqual(got,want) { t.Fatalf("selection=%v",got) }
	p.toggleAll()
	if len(p.selection())!=0 { t.Fatal("all must clear") }
	p.toggleAll()
	if got:=p.selection(); !reflect.DeepEqual(got,want) { t.Fatalf("selection=%v",got) }
	p.move(99)
	if p.cursor!=4 || p.offset!=3 { t.Fatalf("cursor=%d offset=%d",p.cursor,p.offset) }
	p.toggle()
	if got:=p.selection(); !reflect.DeepEqual(got,want) { t.Fatal("disabled row selected") }
	p.move(-99)
	p.button=true
	p.toggle()
	if got:=p.selection(); !reflect.DeepEqual(got,want) { t.Fatal("button modified selection") }
}

func TestPickerRendering(t *testing.T) {
	p:=newPicker([]*row{{Brew:"jq",Port:"jq",Status:"READY"},{Brew:"cargo-c",Port:"cargo-c",Status:"REVIEW"},{Brew:"cargo-binstall",Status:"KEEP"}},3)
	p.toggle()
	var out bytes.Buffer
	p.draw(&out,100)
	for _,s:=range []string{"[x] jq","[ ] cargo-c","[-] cargo-binstall","Migrate Selected (1)"} {
		if !strings.Contains(out.String(),s) { t.Fatalf("missing %q",s) }
	}
	if strings.Contains(safeText("foo\x1b[2J"),"\x1b") { t.Fatal("unsafe terminal escape") }
}
