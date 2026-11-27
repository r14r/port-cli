package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

//go:embed VERSION
var version string

const help = "Usage: port-cli update [--crontab] | migrate [--dry-run | --yes | --ui] [--only NAME] [--exclude NAME] | --version\n"

type runner interface {
	Query(name string, args ...string) (string, error)
	Run(name string, args ...string) error
}

type systemRunner struct{ log io.Writer }

func (r systemRunner) Query(name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	out, err := c.Output()
	if err != nil {
		var e *exec.ExitError
		if errors.As(err, &e) {
			return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(e.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

func (r systemRunner) Run(name string, args ...string) error {
	fmt.Fprintf(r.log, "    $ %s %s\n", name, strings.Join(args, " "))
	c := exec.Command(name, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, r.log, io.MultiWriter(os.Stderr, r.log)
	return c.Run()
}

type manager struct {
	r runner
	brew, port, sudo string
}

type names []string
func (n *names) String() string { return strings.Join(*n, ",") }
func (n *names) Set(v string) error {
	if strings.TrimSpace(v) == "" { return errors.New("empty package name") }
	*n = append(*n, v)
	return nil
}

type options struct {
	yes, dry, ui bool
	only, exclude names
}

func parseOptions(args []string) (options, error) {
	var o options
	f := flag.NewFlagSet("migrate", flag.ContinueOnError)
	f.BoolVar(&o.yes, "yes", false, "execute safe migrations")
	f.BoolVar(&o.dry, "dry-run", false, "report only (default)")
	f.BoolVar(&o.ui, "ui", false, "interactive terminal migration selector")
	f.Var(&o.only, "only", "select formula (repeatable); explicit review of non-smoke-tested matches")
	f.Var(&o.exclude, "exclude", "retain formula (repeatable)")
	if err := f.Parse(args); err != nil { return o, err }
	if f.NArg() != 0 || (o.yes && o.dry) || (o.ui && (o.yes || o.dry || len(o.only) != 0 || len(o.exclude) != 0)) { return o, errors.New("invalid arguments: --ui must be used alone; choose either --dry-run or --yes for non-interactive migration") }
	return o, nil
}

func findExecutable(name string, preferred ...string) (string, error) {
	for _, p := range preferred {
		if s, err := os.Stat(p); err == nil && s.Mode().IsRegular() && s.Mode().Perm()&0111 != 0 { return p, nil }
	}
	p, err := exec.LookPath(name)
	if err != nil { return "", fmt.Errorf("%s not found; install it first", name) }
	return filepath.Abs(p)
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Print(help)
		return nil
	}
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		fmt.Printf("port-cli %s\n", strings.TrimSpace(version))
		return nil
	}
	if args[0] != "update" && args[0] != "migrate" { return errors.New(help) }
	var o options
	var err error
	if args[0] == "migrate" {
		o, err = parseOptions(args[1:])
		if errors.Is(err, flag.ErrHelp) { return nil }
		if err != nil { return err }
	} else if len(args) != 1 { return errors.New("usage: port-cli update [--crontab]") }
	if runtime.GOOS != "darwin" { return errors.New("package operations require macOS") }
	if os.Geteuid() == 0 { return errors.New("run as your normal user; only MacPorts commands use sudo") }
	for _, key := range []string{"HOMEBREW_NO_AUTO_UPDATE", "HOMEBREW_NO_AUTOREMOVE", "HOMEBREW_NO_ANALYTICS", "HOMEBREW_NO_ASK"} {
		if err := os.Setenv(key, "1"); err != nil { return err }
	}
	if err := os.Setenv("LC_ALL", "C"); err != nil { return err }
	port, err := findExecutable("port", "/opt/local/bin/port")
	if err != nil { return err }
	m := manager{r: systemRunner{log: os.Stdout}, port: port, sudo: "/usr/bin/sudo"}
	if args[0] == "update" { return m.update() }
	m.brew, err = findExecutable("brew", "/usr/local/bin/brew", "/opt/homebrew/bin/brew")
	if err != nil { return err }
	if o.ui { return m.migrateUI() }
	return m.migrate(o)
}

func (m manager) update() error {
	fmt.Println("[1/2] Updating MacPorts")
	if err := m.r.Run(m.sudo, m.port, "selfupdate"); err != nil { return err }
	fmt.Println("[2/2] Upgrading outdated ports")
	return m.r.Run(m.sudo, m.port, "upgrade", "outdated")
}

func main() {
	if isCronUpdate(os.Args[1:]) {
		os.Exit(cronUpdate())
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "[ERR]", err)
		os.Exit(1)
	}
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" { out = append(out, l) }
	}
	return out
}

func base(s string) string { return filepath.Base(s) }
func contains(ss []string, name string) bool {
	for _, s := range ss { if s == name || s == base(name) { return true } }
	return false
}

var validName = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_.+@-]*$")
var sensitive = regexp.MustCompile("(?i)^(python.*|node.*|npm|nvm|ruby.*|perl.*|php.*|openjdk.*|go(@.*)?|rust.*|llvm.*|gcc.*|openssl.*|postgresql.*|mysql.*|mariadb.*|redis.*|valkey.*|mongodb.*|sqlite.*|nginx.*|httpd.*|caddy.*|docker.*|colima|lima|qemu.*|ollama|ffmpeg.*|bash|zsh|fish|homebrew.*|brew|macports|ports-cli|port-cli)$")

var aliases = map[string]string{
	"pkg-config":"pkgconfig", "gnu-sed":"gsed", "gnu-tar":"gnutar", "gnu-getopt":"getopt",
	"python@3.10":"python310", "python@3.11":"python311", "python@3.12":"python312",
	"python@3.13":"python313", "python@3.14":"python314", "python@3.15":"python315",
	"openssl@3":"openssl3", "openssl@1.1":"openssl11",
	"node@20":"nodejs20", "node@22":"nodejs22", "node@24":"nodejs24", "openjdk@21":"openjdk21",
}

var smoke = map[string]string{
	"llama.cpp":"llama-server", "jq":"jq", "git":"git", "gh":"gh", "just":"just",
	"ripgrep":"rg", "wget":"wget", "cmake":"cmake", "gsed":"gsed", "gnutar":"gnutar",
	"fd":"fd", "bat":"bat", "fzf":"fzf", "tree":"tree", "yq":"yq",
}

type formula struct {
	Name string `json:"name"`
	FullName string `json:"full_name"`
	KegOnly bool `json:"keg_only"`
	Service json.RawMessage `json:"service"`
}
type inventory struct { Formulae []formula `json:"formulae"` }
type service struct { Name string `json:"name"` }

type row struct {
	Brew, Port, Version, Status, Reason string
	Active bool
	Dependents []string
}

func (m manager) brewLines(args ...string) ([]string, error) {
	s, err := m.r.Query(m.brew, args...)
	return lines(s), err
}

func (m manager) services() ([]string, error) {
	s, err := m.r.Query(m.brew, "services", "list", "--json")
	if err != nil { return nil, err }
	var list []service
	if err := json.Unmarshal([]byte(s), &list); err != nil || list == nil { return nil, errors.New("invalid brew services inventory") }
	var out []string
	for _, v := range list {
		if v.Name == "" { return nil, errors.New("invalid brew service name") }
		out = append(out, base(v.Name))
	}
	return out, nil
}

func (m manager) dependents(name string) ([]string, error) {
	return m.brewLines("uses", "--installed", "--recursive", "--include-optional", name)
}

type portInfo struct { Name, Version string }
func catalog(s string) map[string]portInfo {
	out := map[string]portInfo{}
	for _, l := range lines(s) {
		f := strings.Fields(l)
		if len(f) >= 2 && validName.MatchString(f[0]) && strings.HasPrefix(f[1], "@") {
			out[strings.ToLower(f[0])] = portInfo{f[0], strings.TrimPrefix(f[1], "@")}
		}
	}
	return out
}

func activePorts(s string) map[string]bool {
	out := map[string]bool{}
	for _, l := range lines(s) {
		f := strings.Fields(l)
		if len(f) >= 3 && strings.HasPrefix(f[1], "@") && f[len(f)-1] == "(active)" {
			out[strings.ToLower(f[0])] = true
		}
	}
	return out
}

func (m manager) isActive(name string) (bool, error) {
	s, err := m.r.Query(m.port, "installed")
	return activePorts(s)[strings.ToLower(name)], err
}

// schedule dependents before dependencies; keep cycles and any dependency of
// an excluded, unknown, failed or protected package blocked.
func schedule(rows []*row) []*row {
	var order []*row
	done := map[*row]bool{}
	for {
		changed := false
		for _, r := range rows {
			if r.Status != "CANDIDATE" { continue }
			ready := true
			for _, dep := range r.Dependents {
				var matches []*row
				for _, x := range rows { if x.Brew == dep { matches = append(matches, x) } }
				if len(matches) == 0 {
					for _, x := range rows { if base(x.Brew) == base(dep) { matches = append(matches, x) } }
				}
				if len(matches) != 1 || !done[matches[0]] { ready = false; break }
			}
			if !ready { continue }
			r.Status, r.Reason = "READY", "install, verify, then remove"
			if r.Active { r.Reason = "active port; verify before removal" }
			done[r] = true
			order = append(order, r)
			changed = true
		}
		if !changed { break }
	}
	for _, r := range rows {
		if r.Status == "CANDIDATE" { r.Status, r.Reason = "BLOCKED", "retained dependents: " + strings.Join(r.Dependents, ", ") }
	}
	return order
}

func (m manager) plan(o options) ([]*row, []*row, string, string, error) {
	raw, err := m.r.Query(m.brew, "info", "--json=v2", "--installed")
	if err != nil { return nil, nil, "", "", err }
	var inv inventory
	if err := json.Unmarshal([]byte(raw), &inv); err != nil || inv.Formulae == nil { return nil, nil, "", "", errors.New("invalid Homebrew formula inventory") }
	formulae, err := m.brewLines("list", "--formula", "--full-name")
	if err != nil { return nil, nil, "", "", err }
	casks, err := m.brewLines("list", "--cask")
	if err != nil { return nil, nil, "", "", err }
	pinned, err := m.brewLines("list", "--pinned")
	if err != nil { return nil, nil, "", "", err }
	services, err := m.services()
	if err != nil { return nil, nil, "", "", err }
	p, err := m.r.Query(m.port, "list")
	if err != nil { return nil, nil, "", "", err }
	ports := catalog(p)
	if len(ports) == 0 { return nil, nil, "", "", errors.New("empty MacPorts catalog; run ports-cli update") }
	installed, err := m.r.Query(m.port, "installed")
	if err != nil { return nil, nil, "", "", err }
	active := activePorts(installed)
	meta := map[string]formula{}
	for _, f := range inv.Formulae {
		key := f.FullName
		if key == "" { key = f.Name }
		meta[key] = f
	}
	for _, n := range o.only {
		found := false
		for _, f := range formulae { if n == f || n == base(f) { found = true; break } }
		if !found { return nil, nil, "", "", fmt.Errorf("not installed: %s", n) }
	}
	sort.Strings(formulae)
	sort.Strings(casks)
	var rows []*row
	for _, f := range formulae {
		n := base(f)
		target, mapped := aliases[n]
		if !mapped { target = n }
		port := ports[strings.ToLower(target)]
		r := &row{Brew:f, Port:port.Name, Version:port.Version, Status:"KEEP", Active:active[strings.ToLower(port.Name)]}
		info, ok := meta[f]
		if !ok && !strings.Contains(f, "/") { info, ok = meta[n] }
		switch {
		case len(o.only) != 0 && !contains(o.only, f) || contains(o.exclude, f):
			r.Reason = "not selected or excluded"
		case !validName.MatchString(n) || !ok:
			r.Reason = "unverified formula metadata"
		case contains(pinned, f) || contains(services, f) || sensitive.MatchString(n) || (len(info.Service) != 0 && string(info.Service) != "null"):
			r.Reason = "pinned, service or sensitive runtime"
		case info.KegOnly:
			r.Reason = "keg-only: manual ABI review"
		case strings.Contains(f, "/") && !strings.HasPrefix(f, "homebrew/core/"):
			r.Reason = "third-party tap: manual identity review"
		case strings.Contains(n, "@") && !mapped:
			r.Reason = "versioned formula: manual review"
		case port.Name == "":
			r.Reason = "no matching port"
		case smoke[port.Name] == "" && len(o.only) == 0 && !o.ui:
			r.Status, r.Reason = "REVIEW", "matching port; use --only after reviewing compatibility"
		default:
			r.Status = "CANDIDATE"
			r.Dependents, err = m.dependents(f)
			if err != nil { r.Status, r.Reason = "KEEP", "dependency check failed: " + err.Error() }
		}
		rows = append(rows, r)
	}
	for _, c := range casks {
		p := ports[strings.ToLower(c)]
		rows = append(rows, &row{Brew:c, Port:p.Name, Version:p.Version, Status:"CASK", Reason:"manual GUI migration"})
	}
	return rows, schedule(rows), raw, installed, nil
}

func (m manager) removalGuard(r *row) error {
	deps, err := m.dependents(r.Brew)
	if err != nil { return err }
	if len(deps) != 0 { return fmt.Errorf("still required by %s", strings.Join(deps, ", ")) }
	pinned, err := m.brewLines("list", "--pinned")
	if err != nil { return err }
	services, err := m.services()
	if err != nil { return err }
	if contains(pinned, r.Brew) || contains(services, r.Brew) { return errors.New("now pinned or a service") }
	return nil
}

func (m manager) applyOne(r *row) error {
	if r.Status != "READY" || !validName.MatchString(r.Port) || !validName.MatchString(base(r.Brew)) || strings.HasPrefix(r.Brew, "-") {
		return errors.New("invalid or unapproved migration row")
	}
	if err := m.removalGuard(r); err != nil { return err }
	active, err := m.isActive(r.Port)
	if err != nil { return err }
	if !active {
		if err := m.r.Run(m.sudo, m.port, "install", r.Port); err != nil { return fmt.Errorf("port install failed: %w", err) }
	}
	active, err = m.isActive(r.Port)
	if err != nil { return err }
	if !active { return errors.New("replacement port is not active") }
	if binary := smoke[r.Port]; binary != "" {
		if err := m.r.Run(filepath.Join(filepath.Dir(m.port), binary), "--version"); err != nil { return fmt.Errorf("replacement smoke test failed: %w", err) }
	}
	if err := m.removalGuard(r); err != nil { return err }
	if err := m.r.Run(m.brew, "uninstall", "--formula", r.Brew); err != nil { return err }
	remaining, err := m.brewLines("list", "--formula", "--full-name")
	if err != nil { return err }
	for _, f := range remaining { if f == r.Brew { return errors.New("Homebrew formula remains installed") } }
	return nil
}
