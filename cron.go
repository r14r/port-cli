package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

func isCronUpdate(args []string) bool {
	return len(args) == 2 && args[0] == "update" && args[1] == "--crontab"
}

// runCron opens a separate private log for each invocation, even if an
// overlapping cron-mode update is already holding the advisory lock.
func runCron(home string, update func(io.Writer) error) (int, error) {
	if home == "" || !filepath.IsAbs(home) {
		return 1, fmt.Errorf("HOME must be an absolute path")
	}
	dir := filepath.Join(home, "logs", "port-cli")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return 1, err
	}
	log, err := os.CreateTemp(dir, "update-"+time.Now().Format("20060102-150405")+"-*.log")
	if err != nil {
		return 1, err
	}
	defer log.Close()

	fmt.Fprintf(log, "[START] %s port-cli update --crontab\n", time.Now().Format(time.RFC3339))
	result := func() error {
		lock, err := os.OpenFile(filepath.Join(dir, ".update.lock"), os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		defer lock.Close()
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			return fmt.Errorf("cannot acquire update lock (another cron update may be running): %w", err)
		}
		// Closing the descriptor releases the lock, including on process exit.
		return update(log)
	}()

	code := 0
	if result != nil {
		code = 1
		fmt.Fprintf(log, "[ERR] %v\n", result)
	} else {
		fmt.Fprintln(log, "[OK] MacPorts update completed")
	}
	fmt.Fprintf(log, "[END] %s exit=%d\n", time.Now().Format(time.RFC3339), code)
	if err := log.Sync(); err != nil {
		return 1, err
	}
	return code, nil
}

func cronCommands(w io.Writer, run func(string, ...string) error) error {
	const port = "/opt/local/bin/port"
	for i, args := range [][]string{
		{"-n", port, "-N", "selfupdate"},
		{"-n", port, "-N", "upgrade", "outdated"},
	} {
		fmt.Fprintf(w, "[%d/2] /usr/bin/sudo", i+1)
		for _, arg := range args {
			fmt.Fprintf(w, " %s", arg)
		}
		fmt.Fprintln(w)
		if err := run("/usr/bin/sudo", args...); err != nil {
			return fmt.Errorf("step %d failed: %w (check MacPorts and non-interactive sudo authorization)", i+1, err)
		}
	}
	return nil
}

func cronUpdate() int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERR] Cannot resolve home for cron logging:", err)
		return 1
	}
	code, err := runCron(home, func(w io.Writer) error {
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("package operations require macOS")
		}
		if os.Geteuid() == 0 {
			return fmt.Errorf("run cron mode as your normal user")
		}
		if s, err := os.Stat("/opt/local/bin/port"); err != nil || !s.Mode().IsRegular() || s.Mode().Perm()&0111 == 0 {
			return fmt.Errorf("MacPorts executable missing or not executable: /opt/local/bin/port")
		}
		if err := os.Setenv("PATH", "/opt/local/bin:/opt/local/sbin:/usr/bin:/bin:/usr/sbin:/sbin"); err != nil {
			return err
		}
		if err := os.Setenv("LC_ALL", "C"); err != nil {
			return err
		}
		return cronCommands(w, func(name string, args ...string) error {
			cmd := exec.Command(name, args...)
			// A nil Stdin connects to the null device, not the terminal.
			cmd.Stdout, cmd.Stderr = w, w
			return cmd.Run()
		})
	})
	if err != nil {
		// Only failures of logging itself reach stderr; otherwise cron is silent.
		fmt.Fprintln(os.Stderr, "[ERR] Cron logging failed:", err)
		return 1
	}
	return code
}
