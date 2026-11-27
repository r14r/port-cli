package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// selectedExactly deliberately avoids matching another tap's basename.
func selectedExactly(selected []string,name string) bool {
	for _,s:=range selected {
		if s==name { return true }
	}
	return false
}

// selectedPlan clones the initial UI scan. Scheduling only selected rows
// preserves dependency closure without querying either package manager.
func selectedPlan(snapshot []*row,selected []string) ([]*row,[]*row,error) {
	want:=map[string]bool{}
	for _,name:=range selected {
		if want[name] { return nil,nil,fmt.Errorf("duplicate selection: %s",name) }
		want[name]=true
	}
	var rows []*row
	found:=map[string]bool{}
	for _,original:=range snapshot {
		r:=*original
		r.Dependents=append([]string(nil),original.Dependents...)
		if want[r.Brew] && selectable(original) {
			if found[r.Brew] { return nil,nil,fmt.Errorf("ambiguous selection: %s",r.Brew) }
			found[r.Brew]=true
			r.Status="CANDIDATE"
			r.Reason=""
		} else if r.Status!="CASK" {
			r.Status="KEEP"
			r.Reason="not selected or protected"
		}
		rows=append(rows,&r)
	}
	for name:=range want {
		if !found[name] { return nil,nil,fmt.Errorf("package not eligible in snapshot: %s",name) }
	}
	return rows,schedule(rows),nil
}

// Ruby single-quoted strings avoid interpolation in the generated Brewfile.
func rubyQuote(s string) string {
	s=strings.ReplaceAll(s,"\\","\\\\")
	s=strings.ReplaceAll(s,"'","\\'")
	return "'"+s+"'"
}

func packageBrewfile(rows []*row) string {
	taps:=map[string]bool{}
	var formulae,casks []string
	for _,r:=range rows {
		parts:=strings.Split(r.Brew,"/")
		if len(parts)==3 { taps[strings.Join(parts[:2],"/")]=true }
		if r.Status=="CASK" { casks=append(casks,"cask "+rubyQuote(r.Brew)) } else { formulae=append(formulae,"brew "+rubyQuote(r.Brew)) }
	}
	var tapLines []string
	for t:=range taps { tapLines=append(tapLines,"tap "+rubyQuote(t)) }
	sort.Strings(tapLines)
	sort.Strings(formulae)
	sort.Strings(casks)
	return "# Package-only recovery inventory; not an exact rollback or application-data backup.\n"+strings.Join(append(append(tapLines,formulae...),casks...),"\n")+"\n"
}

type versionSnapshot struct {
	Formulae []struct {
		Name string
		FullName string `json:"full_name"`
		Installed []struct { Version string }
	}
}

func writeRecoveryInventory(dir string,rows []*row,raw,installed string) error {
	var inv versionSnapshot
	if err:=json.Unmarshal([]byte(raw),&inv);err!=nil || inv.Formulae==nil { return fmt.Errorf("invalid cached formula inventory") }
	versions:=map[string][]string{}
	for _,f:=range inv.Formulae {
		name:=f.FullName
		if name=="" { name=f.Name }
		for _,v:=range f.Installed { if v.Version!="" { versions[name]=append(versions[name],v.Version) } }
	}
	var formulae,casks,versionLines []string
	for _,r:=range rows {
		if r.Status=="CASK" { casks=append(casks,r.Brew);continue }
		formulae=append(formulae,r.Brew)
		v:=versions[r.Brew]
		if len(v)==0 { v=[]string{"(unknown in cached metadata)"} }
		versionLines=append(versionLines,r.Brew+" "+strings.Join(v," "))
	}
	sort.Strings(formulae)
	sort.Strings(casks)
	sort.Strings(versionLines)
	plan,err:=json.MarshalIndent(rows,"","  ")
	if err!=nil { return err }
	for name,content:=range map[string]string{
		"brew-installed.json":raw,
		"brew-formulae.txt":strings.Join(formulae,"\n")+"\n",
		"brew-casks.txt":strings.Join(casks,"\n")+"\n",
		"brew-versions.txt":strings.Join(versionLines,"\n")+"\n",
		"ports-before.txt":installed,
		"plan.json":string(plan),
		"Brewfile":packageBrewfile(rows),
	} {
		if err:=os.WriteFile(filepath.Join(dir,name),[]byte(content),0600);err!=nil { return fmt.Errorf("write %s: %w",name,err) }
	}
	return nil
}
