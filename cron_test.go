package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIsCronUpdate(t *testing.T) {
	if !isCronUpdate([]string{"update", "--crontab"}) ||
		isCronUpdate([]string{"update"}) ||
		isCronUpdate([]string{"migrate", "--crontab"}) ||
		isCronUpdate([]string{"update", "--crontab", "--yes"}) {
		t.Fatal("unexpected cron argument handling")
	}
}

func TestCronCommands(t *testing.T) {
	want := []string{
		"/usr/bin/sudo -n /opt/local/bin/port -N selfupdate",
		"/usr/bin/sudo -n /opt/local/bin/port -N upgrade outdated",
	}
	for _, fail := range []bool{false, true} {
		var got []string
		var log bytes.Buffer
		err := cronCommands(&log, func(name string, args ...string) error {
			got = append(got, name+" "+strings.Join(args, " "))
			if fail {
				return errors.New("sudo password required")
			}
			return nil
		})
		expected := want
		if fail {
			expected = want[:1]
		}
		if !reflect.DeepEqual(got, expected) || (err != nil) != fail {
			t.Fatalf("fail=%v commands=%v err=%v", fail, got, err)
		}
	}
}

func TestCronLogs(t *testing.T) {
	home := t.TempDir()
	for _, fail := range []bool{false, true} {
		code, err := runCron(home, func(w io.Writer) error {
			io.WriteString(w, "command output\n")
			if fail {
				return errors.New("simulated error")
			}
			return nil
		})
		if err != nil || (code != 0) != fail {
			t.Fatalf("code=%d err=%v", code, err)
		}
	}
	paths, err := filepath.Glob(filepath.Join(home, "logs", "port-cli", "update-*.log"))
	if err != nil || len(paths) != 2 {
		t.Fatalf("logs=%v err=%v", paths, err)
	}
	var successes, failures int
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(data)
		if !strings.Contains(s, "[START]") || !strings.Contains(s, "command output") || !strings.Contains(s, "[END]") {
			t.Fatalf("incomplete log: %s", s)
		}
		if strings.Contains(s, "[OK]") && strings.Contains(s, "exit=0") {
			successes++
		}
		if strings.Contains(s, "simulated error") && strings.Contains(s, "exit=1") {
			failures++
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("log must be private")
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("successes=%d failures=%d", successes, failures)
	}
}

func TestCronOverlap(t *testing.T) {
	home := t.TempDir()
	code, err := runCron(home, func(w io.Writer) error {
		inner, err := runCron(home, func(io.Writer) error {
			t.Fatal("overlapping operation must not execute")
			return nil
		})
		if err != nil || inner != 1 {
			t.Fatalf("overlap code=%d err=%v", inner, err)
		}
		return nil
	})
	if err != nil || code != 0 {
		t.Fatalf("outer code=%d err=%v", code, err)
	}
	code, err = runCron(home, func(io.Writer) error { return nil })
	if err != nil || code != 0 {
		t.Fatal("lock was not released")
	}
}
