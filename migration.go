package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func (m manager) migrate(o options) error {
	fmt.Println("Scanning Homebrew inventory and MacPorts catalog...")
	rows, order, raw, installed, err := m.plan(o)
	if err != nil { return err }
	if o.yes { fmt.Println("\nMIGRATION PLAN") } else { fmt.Println("\nDRY RUN — no package changes") }
	fmt.Printf("%-26s %-22s %-9s %s\n", "Homebrew", "MacPorts", "Status", "Reason")
	for _, r := range rows {
		target := r.Port
		if target == "" { target = "-" }
		fmt.Printf("%-26s %-22s %-9s %s\n", r.Brew, target, r.Status, r.Reason)
	}
	fmt.Printf("\nPackages: %d | Ready: %d | Retained: %d\n", len(rows), len(order), len(rows)-len(order))
	fmt.Println("Catalog matches do not guarantee macOS compatibility, ABI or application-data compatibility.")
	if o.ui && len(order) != len(o.only) { return errors.New("selection changed or is blocked; no changes made") }
	if !o.yes || len(order) == 0 { return nil }
	dir, err := m.backup(rows, raw, installed)
	if err != nil { return err }
	log, err := os.OpenFile(filepath.Join(dir, "migration.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil { return err }
	defer log.Close()
	w := io.MultiWriter(os.Stdout, log)
	m.r = systemRunner{log:w}
	fmt.Fprintf(w, "Inventory: %s\n", dir)
	fmt.Fprintln(w, "Inventory only; does not back up application data or package binaries.")
	if err := m.r.Run(m.sudo, "-v"); err != nil { return err }
	failed := 0
	for i, r := range order {
		fmt.Fprintf(w, "\n[%02d/%02d] %s -> %s\n", i+1, len(order), r.Brew, r.Port)
		if err := m.applyOne(r); err != nil {
			failed++
			fmt.Fprintf(w, "    [ERR] %v; inspect both managers before retrying\n", err)
		} else { fmt.Fprintln(w, "    [OK] Migrated") }
	}
	fmt.Fprintf(w, "\nMigrated: %d | Failed: %d | Log: %s\n", len(order)-failed, failed, log.Name())
	if failed > 0 { return errors.New("one or more migrations failed") }
	return nil
}

func (m manager) backup(rows []*row, raw, installed string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil { return "", err }
	root := filepath.Join(home, ".local", "state", "ports-cli")
	if err := os.MkdirAll(root, 0700); err != nil { return "", err }
	dir, err := os.MkdirTemp(root, time.Now().Format("20060102-150405-"))
	if err != nil { return "", err }
	versions, err := m.r.Query(m.brew, "list", "--versions")
	if err != nil { return "", err }
	plan, err := json.MarshalIndent(rows, "", "  ")
	if err != nil { return "", err }
	for name, content := range map[string]string{"brew-installed.json":raw, "ports-before.txt":installed, "brew-versions.txt":versions, "plan.json":string(plan)} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil { return "", err }
	}
	brewfile := filepath.Join(dir, "Brewfile")
	if err := m.r.Run(m.brew, "bundle", "dump", "--file="+brewfile); err != nil { return "", fmt.Errorf("Brewfile snapshot failed: %w", err) }
	if s, err := os.Stat(brewfile); err != nil || !s.Mode().IsRegular() { return "", errors.New("Brewfile snapshot missing") }
	return dir, nil
}
