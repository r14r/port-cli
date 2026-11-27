package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type picker struct {
	rows []*row
	selected map[string]bool
	cursor, offset, page int
	button bool
}

func selectable(r *row) bool {
	return r.Status == "READY" || r.Status == "REVIEW"
}

func newPicker(rows []*row, page int) *picker {
	return &picker{rows: rows, selected: make(map[string]bool), page: page}
}

func (p *picker) move(delta int) {
	p.button = false
	if len(p.rows) == 0 { return }
	p.cursor += delta
	if p.cursor < 0 { p.cursor = 0 }
	if p.cursor >= len(p.rows) { p.cursor = len(p.rows)-1 }
	if p.cursor < p.offset { p.offset = p.cursor }
	if p.cursor >= p.offset+p.page { p.offset = p.cursor-p.page+1 }
}

func (p *picker) toggle() {
	if p.button || len(p.rows) == 0 { return }
	r := p.rows[p.cursor]
	if !selectable(r) { return }
	if p.selected[r.Brew] { delete(p.selected,r.Brew) } else { p.selected[r.Brew] = true }
}

func (p *picker) toggleAll() {
	count := 0
	for _, r := range p.rows { if selectable(r) { count++ } }
	if len(p.selected) == count {
		p.selected = make(map[string]bool)
		return
	}
	for _, r := range p.rows { if selectable(r) { p.selected[r.Brew] = true } }
}

func (p *picker) selection() []string {
	var names []string
	for name := range p.selected { names = append(names,name) }
	sort.Strings(names)
	return names
}

// Do not allow package manager output to inject terminal control sequences.
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r >= 127 && r < 160 { return ' ' }
		return r
	},s)
}

func clipped(s string, width int) string {
	r := []rune(safeText(s))
	if width <= 0 { return "" }
	if len(r) <= width { return string(r) }
	if width == 1 { return string(r[:1]) }
	return string(r[:width-1])+"…"
}

func (p *picker) draw(w io.Writer, width int) {
	fmt.Fprint(w,"\x1b[H\x1b[2J")
	write := func(s string) { fmt.Fprintln(w,clipped(s,width-1)) }
	write("PORT-CLI  |  Select packages to migrate")
	write(fmt.Sprintf("Packages: %d    Selected: %d",len(p.rows),len(p.selected)))
	write("Up/Down: move   Space: select   a: all/none   n/p: page")
	write("Tab: button   Enter: activate   q: cancel")
	nameWidth, portWidth := 24, 22
	if width < 85 { nameWidth, portWidth = 19, 17 }
	reasonWidth := width-nameWidth-portWidth-23
	write(fmt.Sprintf("     %-*s %-*s %-8s %s",nameWidth,"Homebrew",portWidth,"MacPorts","Status","Reason"))
	write(strings.Repeat("-",width-1))
	end := p.offset+p.page
	if end > len(p.rows) { end = len(p.rows) }
	for i:=p.offset; i<end; i++ {
		r:=p.rows[i]
		mark := " "
		if !p.button && i == p.cursor { mark = ">" }
		check := "[ ]"
		if !selectable(r) { check = "[-]" } else if p.selected[r.Brew] { check = "[x]" }
		target := r.Port
		if target == "" { target = "-" }
		line := fmt.Sprintf("%s%s %-*s %-*s %-8s %s",mark,check,nameWidth,clipped(r.Brew,nameWidth),portWidth,clipped(target,portWidth),r.Status,clipped(r.Reason,reasonWidth))
		if !p.button && i == p.cursor { fmt.Fprint(w,"\x1b[7m") }
		fmt.Fprint(w,clipped(line,width-1))
		fmt.Fprintln(w,"\x1b[0m")
	}
	for i := end-p.offset; i < p.page; i++ { fmt.Fprintln(w) }
	write(strings.Repeat("-",width-1))
	if len(p.rows) > 0 {
		r := p.rows[p.cursor]
		write(fmt.Sprintf("%d-%d / %d | %s: %s",p.offset+1,end,len(p.rows),r.Brew,r.Reason))
	} else { write("No installed packages.") }
	button := fmt.Sprintf("[ Migrate Selected (%d) ]",len(p.selected))
	if len(p.selected) == 0 { button += " (disabled)" }
	if p.button { fmt.Fprint(w,"\x1b[7m") }
	fmt.Fprint(w,button)
	fmt.Fprintln(w,"\x1b[0m")
}

func stty(args ...string) (string,error) {
	cmd := exec.Command("/bin/stty",args...)
	cmd.Stdin = os.Stdin
	b,err := cmd.Output()
	return strings.TrimSpace(string(b)),err
}

func choosePackages(rows []*row) ([]string,error) {
	if os.Getenv("TERM") == "" || os.Getenv("TERM") == "dumb" { return nil,errors.New("--ui requires an interactive terminal") }
	for _, f := range []*os.File{os.Stdin,os.Stdout} {
		s,err := f.Stat()
		if err != nil || s.Mode() & os.ModeCharDevice == 0 { return nil,errors.New("--ui requires terminal stdin and stdout") }
	}
	state,err := stty("-g")
	if err != nil { return nil,fmt.Errorf("read terminal state: %w",err) }
	size,err := stty("size")
	if err != nil { return nil,err }
	parts := strings.Fields(size)
	if len(parts)!=2 { return nil,errors.New("cannot determine terminal size") }
	height,e1 := strconv.Atoi(parts[0])
	width,e2 := strconv.Atoi(parts[1])
	if e1!=nil || e2!=nil || width<64 || height<13 { return nil,errors.New("--ui requires a terminal of at least 64 columns and 13 rows") }
	var once sync.Once
	restore := func() { once.Do(func(){
		_,_ = stty(state)
		fmt.Fprint(os.Stdout,"\x1b[0m\x1b[?25h\x1b[?1049l")
	}) }
	defer restore()
	signals := make(chan os.Signal,1)
	done := make(chan struct{})
	signal.Notify(signals,os.Interrupt,syscall.SIGTERM,syscall.SIGHUP)
	defer signal.Stop(signals)
	defer close(done)
	go func(){ select {
	case <-signals: restore(); os.Exit(130)
	case <-done:
	} }()
	if _,err := stty("-icanon","-echo","min","1","time","0"); err != nil { return nil,err }
	fmt.Fprint(os.Stdout,"\x1b[?1049h\x1b[?25l")
	p := newPicker(rows,height-10)
	// Read one byte at a time; do not buffer confirmation input.
	escape := 0
	var b [1]byte
	for {
		p.draw(os.Stdout,width)
		for {
			if _,err := io.ReadFull(os.Stdin,b[:]); err != nil { return nil,err }
			k := b[0]
			if escape == 1 {
				escape = 0
				if k=='[' || k=='O' { escape=2; continue }
			} else if escape==2 {
				escape=0
				if k=='A' { p.move(-1) }
				if k=='B' { p.move(1) }
				break
			}
			switch k {
			case 27: escape=1; continue
			case 'q','Q',3,4: return nil,nil
			case 'j': p.move(1)
			case 'k': p.move(-1)
			case 'n': p.move(p.page)
			case 'p': p.move(-p.page)
			case ' ': p.toggle()
			case 'a': p.toggleAll()
			case '\t': p.button=!p.button
			case '\r','\n':
				if p.button {
					if len(p.selected)>0 { return p.selection(),nil }
				} else { p.toggle() }
			}
			break
		}
	}
}

// Include matches without a predefined executable smoke test when computing
// possible dependency-safe selections, while still disabling protected rows.
func (m manager) migrateUI(details bool) error {
	if !details { fmt.Println("Scanning Homebrew inventory and MacPorts catalog...") }
	rows,_,raw,installed,err := m.plan(options{ui:true,details:details})
	if err != nil { return err }
	for _,r := range rows {
		if r.Status=="READY" && smoke[r.Port]=="" {
			r.Status="REVIEW"
			r.Reason="activation check only; no predefined executable smoke test"
		}
	}
	if details { fmt.Println("        Opening migration selector...") }
	selection,err := choosePackages(rows)
	if err != nil { return err }
	if len(selection)==0 { fmt.Println("Migration cancelled."); return nil }
	o := options{yes:true,ui:true,details:details,only:names(selection)}
	fmt.Println("Checking selected packages and dependencies using cached inventory...")
	checked,order,err := selectedPlan(rows,selection)
	if err != nil { return err }
	if len(order)!=len(selection) {
		fmt.Println("Selection cannot be migrated safely:")
		for _,r := range checked {
			if contains(selection,r.Brew) && r.Status!="READY" {
				fmt.Printf("  %s: %s — %s\n",safeText(r.Brew),r.Status,safeText(r.Reason))
			}
		}
		return errors.New("select the required eligible dependents as well; no changes made")
	}
	fmt.Printf("\nSelected packages (%d), in migration order:\n",len(order))
	for _,r := range order {
		verify := "activation + executable smoke test"
		if smoke[r.Port]=="" { verify="activation only; review functional compatibility" }
		fmt.Printf("  %s -> %s (%s)\n",safeText(r.Brew),safeText(r.Port),verify)
	}
	fmt.Print("\nInstall replacements and remove these Homebrew formulae after verification? [y/N] ")
	answer,err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err,io.EOF) { return err }
	if !strings.EqualFold(strings.TrimSpace(answer),"y") { fmt.Println("Migration cancelled."); return nil }
	return m.executePlan(o,checked,order,raw,installed)
}
