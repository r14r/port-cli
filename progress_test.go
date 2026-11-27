package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type progressRunner struct { failDependency bool }

func (r progressRunner) Run(string, ...string) error { return errors.New("planning must not execute mutations") }
func (r progressRunner) Query(_ string, args ...string) (string,error) {
	switch strings.Join(args, " ") {
	case "info --json=v2 --installed":
		return "{\"formulae\":[{\"name\":\"jq\",\"full_name\":\"jq\"},{\"name\":\"cargo-c\",\"full_name\":\"cargo-c\"},{\"name\":\"cargo-binstall\",\"full_name\":\"cargo-binstall\"}]}",nil
	case "list --formula --full-name": return "jq\ncargo-c\ncargo-binstall\n",nil
	case "list --cask": return "example-app\n",nil
	case "list --pinned": return "",nil
	case "services list --json": return "[]",nil
	case "list": return "jq @1.7 devel\ncargo-c @1.0 devel\nother @2.0 devel\n",nil
	case "installed": return "jq @1.7_0 (active)\n",nil
	case "uses --installed --recursive --include-optional jq":
		if r.failDependency { return "",errors.New("simulated failure") }
		return "",nil
	case "uses --installed --recursive --include-optional cargo-c": return "",nil
	default: return "",errors.New("unexpected query")
	}
}

func TestDetailedMigrationPlan(t *testing.T) {
	for _, details := range []bool{false,true} {
		var out bytes.Buffer
		m := manager{r:progressRunner{},brew:"brew",port:"port"}
		rows,order,_,_,err := m.plan(options{ui:true,details:details,progress:&out})
		if err != nil || len(rows)!=4 || len(order)!=2 { t.Fatalf("rows=%d order=%d err=%v",len(rows),len(order),err) }
		if !details {
			if out.Len()!=0 { t.Fatal("default planning must remain quiet") }
			continue
		}
		for _,s:=range []string{"[01/05]","4 Homebrew packages (3 formulae, 1 casks)","[02/05]","3 catalog entries, 1 active ports","[03/05]","2 catalog matches, 2 potential migration candidates","[04/05]","[1/2] cargo-c","[2/2] jq","[05/05]","READY: 1 | REVIEW: 1 | BLOCKED: 0 | KEEP: 1 | CASK: 1"} {
			if !strings.Contains(out.String(),s) { t.Fatalf("missing %q in %s",s,out.String()) }
		}
	}
}

func TestDetailedDependencyFailure(t *testing.T) {
	var out bytes.Buffer
	m := manager{r:progressRunner{failDependency:true},brew:"brew",port:"port"}
	_,order,_,_,err := m.plan(options{ui:true,details:true,progress:&out})
	if err != nil || len(order)!=1 { t.Fatalf("order=%d err=%v",len(order),err) }
	if !strings.Contains(out.String(),"1 failed (retained) [WARN]") { t.Fatal(out.String()) }
}

func TestDetailsOptions(t *testing.T) {
	for _,args:=range [][]string{{"--ui","--details"},{"--details","--ui"},{"--dry-run","--details"},{"--yes","--details"}} {
		o,err:=parseOptions(args)
		if err!=nil || !o.details { t.Fatalf("args=%v o=%+v err=%v",args,o,err) }
	}
}
