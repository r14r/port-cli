package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelectedPlanReusesSnapshot(t *testing.T) {
	snapshot:=[]*row{
		{Brew:"lib",Port:"lib",Status:"REVIEW",Dependents:[]string{"app"}},
		{Brew:"app",Port:"app",Status:"READY"},
		{Brew:"protected",Status:"KEEP"},
		{Brew:"gui",Status:"CASK"},
	}
	rows,order,err:=selectedPlan(snapshot,[]string{"lib","app"})
	if err!=nil || len(rows)!=4 || len(order)!=2 || order[0].Brew!="app" || order[1].Brew!="lib" { t.Fatalf("rows=%v order=%v err=%v",rows,order,err) }
	if snapshot[0].Status!="REVIEW" || snapshot[1].Status!="READY" { t.Fatal("original scan mutated") }
	_,order,err=selectedPlan(snapshot,[]string{"lib"})
	if err!=nil || len(order)!=0 { t.Fatal("missing dependent was not blocked") }
	for _,name:=range []string{"protected","gui","unknown"} {
		if _,_,err:=selectedPlan(snapshot,[]string{name});err==nil { t.Fatalf("accepted %s",name) }
	}
}

func TestRecoveryInventoryWithoutBundle(t *testing.T) {
	dir:=t.TempDir()
	rows:=[]*row{
		{Brew:"bat",Port:"bat",Status:"READY"},
		{Brew:"openssl@1.1",Status:"KEEP"},
		{Brew:"paradoxinfinite/oriel/oriel",Status:"CASK"},
	}
	raw:="{\"formulae\":[{\"name\":\"bat\",\"full_name\":\"bat\",\"installed\":[{\"version\":\"0.25.0\"}]}]}"
	if err:=writeRecoveryInventory(dir,rows,raw,"bat @0.25_0 (active)");err!=nil { t.Fatal(err) }
	for name,expected:=range map[string][]string{
		"Brewfile":{"brew 'bat'","brew 'openssl@1.1'","tap 'paradoxinfinite/oriel'","cask 'paradoxinfinite/oriel/oriel'"},
		"brew-versions.txt":{"bat 0.25.0","openssl@1.1 (unknown"},
		"brew-formulae.txt":{"bat","openssl@1.1"},
		"brew-casks.txt":{"paradoxinfinite/oriel/oriel"},
		"brew-installed.json":{raw},
		"ports-before.txt":{"bat @0.25_0 (active)"},
		"plan.json":{"bat"},
	} {
		data,err:=os.ReadFile(filepath.Join(dir,name))
		if err!=nil { t.Fatal(err) }
		for _,s:=range expected { if !strings.Contains(string(data),s) { t.Fatalf("%s missing %s",name,s) } }
	}
}
