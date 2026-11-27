package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestOptions(t *testing.T) {
	o, err := parseOptions([]string{"--only", "jq", "--only", "git", "--yes"})
	if err != nil || !o.yes || len(o.only) != 2 { t.Fatalf("%+v %v", o, err) }
	if _, err := parseOptions([]string{"--yes", "--dry-run"}); err == nil { t.Fatal("conflicting flags accepted") }
	o, err = parseOptions(nil)
	if err != nil || o.yes { t.Fatal("default must not mutate") }
}

func TestCatalog(t *testing.T) {
	p := catalog("jq @1.7.1 devel\nllama.cpp @0.1 science\nbad line\n")
	if p["jq"].Version != "1.7.1" || p["llama.cpp"].Name != "llama.cpp" || len(p) != 2 { t.Fatal(p) }
	a := activePorts("The following ports are currently installed:\n  jq @1.7_0 (active)\n  jq @1.6_0\n  git @1_0\n")
	if !a["jq"] || a["git"] { t.Fatal(a) }
}

func TestSchedule(t *testing.T) {
	lib := &row{Brew:"lib", Status:"CANDIDATE", Dependents:[]string{"app"}}
	app := &row{Brew:"app", Status:"CANDIDATE"}
	block := &row{Brew:"block", Status:"CANDIDATE", Dependents:[]string{"retained"}}
	retained := &row{Brew:"retained", Status:"KEEP"}
	order := schedule([]*row{lib, app, block, retained})
	if !reflect.DeepEqual(order, []*row{app,lib}) || block.Status != "BLOCKED" { t.Fatalf("%v %v", order, block) }
}

type fakeRunner struct {
	commands []string
	fail string
	active bool
	deps string
}

func (f *fakeRunner) Query(name string, args ...string) (string, error) {
	switch strings.Join(args, " ") {
	case "installed":
		if f.active { return "jq @1.7_0 (active)\n", nil }
		return "None of the specified ports are installed.\n", nil
	case "uses --installed --recursive --include-optional jq": return f.deps, nil
	case "list --pinned", "list --formula --full-name": return "", nil
	case "services list --json": return "[]", nil
	}
	return "", errors.New("unexpected query")
}

func (f *fakeRunner) Run(name string, args ...string) error {
	s := name + " " + strings.Join(args," ")
	f.commands = append(f.commands,s)
	if f.fail != "" && strings.Contains(s, f.fail) { return errors.New("simulated failure") }
	if strings.Contains(s, "install jq") && !strings.Contains(s,"uninstall") { f.active = true }
	return nil
}

func TestMigrationSafety(t *testing.T) {
	for _, tc := range []struct{ name, fail, deps string; wantErr bool; want []string }{
		{"success","","",false,[]string{"sudo /opt/local/bin/port install jq","/opt/local/bin/jq --version","brew uninstall --formula jq"}},
		{"install fails","install jq","",true,[]string{"sudo /opt/local/bin/port install jq"}},
		{"smoke fails","--version","",true,[]string{"sudo /opt/local/bin/port install jq","/opt/local/bin/jq --version"}},
		{"retained dependent","","app",true,nil},
	} {
		t.Run(tc.name,func(t *testing.T){
			f := &fakeRunner{fail:tc.fail,deps:tc.deps}
			m := manager{r:f,brew:"brew",port:"/opt/local/bin/port",sudo:"sudo"}
			err := m.applyOne(&row{Brew:"jq",Port:"jq",Status:"READY"})
			if (err != nil) != tc.wantErr || !reflect.DeepEqual(f.commands,tc.want) { t.Fatalf("err=%v commands=%v",err,f.commands) }
		})
	}
}

func TestUpdateStopsOnFailure(t *testing.T) {
	f := &fakeRunner{fail:"selfupdate"}
	m := manager{r:f,port:"port",sudo:"sudo"}
	if err := m.update(); err == nil || len(f.commands) != 1 { t.Fatalf("%v %v",err,f.commands) }
}
