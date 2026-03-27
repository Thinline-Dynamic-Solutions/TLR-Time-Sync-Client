// tlr-time-sync — keeps an SDR-Trunk machine's clock in sync with a TLR server.
//
// Usage:
//
//	tlr-time-sync install   [-config path]   install and start as a system service
//	tlr-time-sync uninstall                  stop and remove the service
//	tlr-time-sync start                      start an already-installed service
//	tlr-time-sync stop                       stop a running service
//	tlr-time-sync run       [-config path]   run in the foreground (debug)
//
// Windows: installs as a Windows Service running as LocalSystem (has clock rights).
// Linux:   installs as a systemd unit running as root.
// macOS:   installs as a LaunchDaemon running as root.
//
// The install/uninstall/start/stop commands require Administrator / root once.
// After that the service starts automatically at boot with no further prompts.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kardianos/service"
	"gopkg.in/ini.v1"
)

const Version = "1.0.0"

// ── config ────────────────────────────────────────────────────────────────────

type config struct {
	ServerURL        string
	IntervalSeconds  int
	FailureThreshold int
	Samples          int
}

func loadConfig(path string) (*config, error) {
	cfg, err := ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", path, err)
	}

	c := &config{
		ServerURL:        cfg.Section("server").Key("url").String(),
		IntervalSeconds:  cfg.Section("sync").Key("interval_seconds").MustInt(30),
		FailureThreshold: cfg.Section("sync").Key("failure_threshold").MustInt(5),
		Samples:          cfg.Section("sync").Key("samples").MustInt(4),
	}
	if c.Samples < 1 {
		c.Samples = 1
	}
	if c.Samples > 10 {
		c.Samples = 10
	}

	c.ServerURL = strings.TrimRight(c.ServerURL, "/")
	if c.ServerURL == "" {
		return nil, fmt.Errorf("server.url is required in the config file")
	}
	if c.IntervalSeconds < 10 {
		log.Printf("interval_seconds %d is below minimum 10 — using 10", c.IntervalSeconds)
		c.IntervalSeconds = 10
	}
	return c, nil
}

// ── time response ─────────────────────────────────────────────────────────────

type timeResponse struct {
	UnixNS int64 `json:"unix_ns"`
}

// ── rate limiter ──────────────────────────────────────────────────────────────

type rateLimiter struct {
	minInterval  time.Duration
	lastRequest  time.Time
	failures     int
	backoffUntil time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{minInterval: time.Second}
}

func (r *rateLimiter) wait() {
	now := time.Now()
	if now.Before(r.backoffUntil) {
		sleep := time.Until(r.backoffUntil)
		log.Printf("rate limiter: backing off for %s", sleep.Round(time.Second))
		time.Sleep(sleep)
	}
	since := time.Since(r.lastRequest)
	if since < r.minInterval {
		time.Sleep(r.minInterval - since)
	}
}

func (r *rateLimiter) success() {
	r.failures = 0
	r.backoffUntil = time.Time{}
	r.lastRequest = time.Now()
}

func (r *rateLimiter) failure(threshold int) {
	r.failures++
	r.lastRequest = time.Now()
	if r.failures >= threshold {
		exp := r.failures - threshold
		if exp > 6 {
			exp = 6
		}
		backoff := time.Duration(5<<uint(exp)) * time.Second
		if backoff > 10*time.Minute {
			backoff = 10 * time.Minute
		}
		r.backoffUntil = time.Now().Add(backoff)
		log.Printf("rate limiter: %d consecutive failures — backing off %s", r.failures, backoff)
	}
}

// ── HTTP client ───────────────────────────────────────────────────────────────

var httpClient = &http.Client{Timeout: 5 * time.Second}

func queryServerTime(url string) (t1, t3 time.Time, serverNS int64, err error) {
	t1 = time.Now()

	resp, err := httpClient.Get(url)
	if err != nil {
		return t1, t1, 0, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return t1, t1, 0, fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return t1, t1, 0, fmt.Errorf("reading response: %w", err)
	}

	t3 = time.Now()

	var tr timeResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return t1, t3, 0, fmt.Errorf("parsing response: %w", err)
	}
	if tr.UnixNS == 0 {
		return t1, t3, 0, fmt.Errorf("server returned zero timestamp")
	}

	return t1, t3, tr.UnixNS, nil
}

// ── clock adjustment ──────────────────────────────────────────────────────────

type sample struct {
	rtt    time.Duration
	offset time.Duration
}

// bestSample takes n samples and returns the one with the lowest RTT.
// The minimum-RTT sample has the least network asymmetry so the NTP midpoint
// estimate of the server timestamp is most accurate.
func bestSample(endpoint string, n int) (sample, error) {
	var best sample
	bestRTT := time.Duration(1<<63 - 1)

	for i := 0; i < n; i++ {
		if i > 0 {
			time.Sleep(20 * time.Millisecond)
		}

		t1, t3, serverNS, err := queryServerTime(endpoint)
		if err != nil {
			return best, err
		}

		rtt := t3.Sub(t1)
		mid := t1.Add(rtt / 2)
		serverTime := time.Unix(0, serverNS).UTC()
		offset := serverTime.Sub(mid)

		if rtt < bestRTT {
			bestRTT = rtt
			best = sample{rtt: rtt, offset: offset}
		}
	}
	return best, nil
}

// applyOffset applies the clock correction.
// Offsets within half the RTT are measurement noise and are skipped.
func applyOffset(s sample) error {
	noise := s.rtt / 2
	abs := s.offset
	if abs < 0 {
		abs = -abs
	}

	if abs <= noise {
		log.Printf("rtt=%s  offset=%s — within measurement noise (noise floor ±%s), skipping",
			s.rtt, s.offset, noise)
		return nil
	}

	corrected := time.Now().Add(s.offset)
	log.Printf("rtt=%s  offset=%s — adjusting system clock to %s",
		s.rtt, s.offset, corrected.Format(time.RFC3339Nano))

	return setSystemTime(corrected)
}

func isPermissionError(err error) bool {
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.EACCES || errno == syscall.EPERM
	}
	return false
}

// ── service ───────────────────────────────────────────────────────────────────

type program struct {
	cfg      *config
	endpoint string
	stop     chan struct{}
}

func (p *program) Start(s service.Service) error {
	p.stop = make(chan struct{})
	go p.run()
	return nil
}

func (p *program) run() {
	interval := time.Duration(p.cfg.IntervalSeconds) * time.Second
	rl := newRateLimiter()

	log.Printf("tlr-time-sync running — server: %s  interval: %s  samples: %d",
		p.cfg.ServerURL, interval, p.cfg.Samples)

	sync := func() {
		rl.wait()
		s, err := bestSample(p.endpoint, p.cfg.Samples)
		if err != nil {
			log.Printf("time query failed: %v", err)
			rl.failure(p.cfg.FailureThreshold)
			return
		}
		if err := applyOffset(s); err != nil {
			if isPermissionError(err) {
				log.Printf("ERROR: permission denied setting system clock — service must run as root/LocalSystem. Error: %v", err)
			} else {
				log.Printf("clock adjustment failed: %v", err)
			}
			rl.failure(p.cfg.FailureThreshold)
			return
		}
		rl.success()
	}

	sync()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sync()
		case <-p.stop:
			return
		}
	}
}

func (p *program) Stop(s service.Service) error {
	close(p.stop)
	return nil
}

// ── main ──────────────────────────────────────────────────────────────────────

// exeDir returns the directory containing the running executable.
// Used to resolve relative config paths so the service (which runs as
// LocalSystem with CWD = C:\Windows\System32) can still find the ini file.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func main() {
	// Default config path is next to the exe, not the working directory.
	defaultConfig := filepath.Join(exeDir(), "tlr-time-sync.ini")
	configPath := flag.String("config", defaultConfig, "path to config file")
	flag.Parse()

	args := flag.Args()
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}

	// For the double-click / interactive path we handle config errors ourselves
	// so we can show a readable message before the window closes.
	// For all other paths, load config and fatal on error as normal.
	loadCfgOrExit := func() *config {
		cfg, err := loadConfig(*configPath)
		if err != nil {
			fmt.Println("Configuration error:", err)
			fmt.Printf("Make sure %s exists and contains a valid server URL.\n", *configPath)
			waitForKey()
			os.Exit(1)
		}
		return cfg
	}

	// uninstall / start / stop don't need the config at all.
	switch cmd {
	case "uninstall":
		svc, _ := service.New(&program{}, &service.Config{Name: "TLRTimeSync"})
		_ = service.Control(svc, "stop")
		if err := service.Control(svc, "uninstall"); err != nil {
			fmt.Println("Uninstall failed:", err)
			waitForKey()
			os.Exit(1)
		}
		fmt.Println("TLR Time Sync service removed.")
		waitForKey()
		return

	case "start":
		svc, _ := service.New(&program{}, &service.Config{Name: "TLRTimeSync"})
		if err := service.Control(svc, "start"); err != nil {
			fmt.Println("Start failed:", err)
			waitForKey()
			os.Exit(1)
		}
		fmt.Println("Service started.")
		waitForKey()
		return

	case "stop":
		svc, _ := service.New(&program{}, &service.Config{Name: "TLRTimeSync"})
		if err := service.Control(svc, "stop"); err != nil {
			fmt.Println("Stop failed:", err)
			waitForKey()
			os.Exit(1)
		}
		fmt.Println("Service stopped.")
		waitForKey()
		return
	}

	// All remaining paths (install, default/double-click, run-as-service) need config.
	cfg := loadCfgOrExit()

	svcConfig := &service.Config{
		Name:        "TLRTimeSync",
		DisplayName: "TLR Time Sync",
		Description: "Keeps this machine's clock synchronised with the TLR server for accurate call timestamps.",
		Arguments:   []string{"-config", *configPath},
	}

	prg := &program{
		cfg:      cfg,
		endpoint: cfg.ServerURL + "/api/time",
	}

	svc, err := service.New(prg, svcConfig)
	if err != nil {
		fmt.Println("Service setup failed:", err)
		waitForKey()
		os.Exit(1)
	}

	// Route log output through the service logger (Event Log / journald / syslog)
	// when running as a service, or stderr when in the foreground.
	if logger, err := svc.Logger(nil); err == nil {
		log.SetOutput(&serviceLogWriter{logger})
	}

	switch cmd {
	case "install":
		installService(svc)
		waitForKey()

	default:
		if service.Interactive() {
			// User double-clicked (or ran from a terminal) — auto-install.
			if needsElevation() {
				fmt.Println("Requesting administrator privileges...")
				if err := relaunchAsAdmin(*configPath); err != nil {
					fmt.Println("Could not request elevation:", err)
					fmt.Println("Please right-click the exe and choose 'Run as administrator'.")
					waitForKey()
					os.Exit(1)
				}
				// Elevated process will handle the install; this one can exit.
				return
			}
			installService(svc)
			waitForKey()
		} else {
			// Launched by the service manager — run normally.
			if err := svc.Run(); err != nil {
				log.Fatalf("service run error: %v", err)
			}
		}
	}
}

// installService installs and starts the service, printing a user-friendly
// result. Called both from the "install" command and the double-click path.
func installService(svc service.Service) {
	if err := service.Control(svc, "install"); err != nil {
		// Already installed — just ensure it's running.
		if strings.Contains(err.Error(), "already") || strings.Contains(err.Error(), "exists") {
			fmt.Println("TLR Time Sync is already installed as a service.")
			_ = service.Control(svc, "start")
			fmt.Println("Service started (or was already running).")
			return
		}
		fmt.Println("Install failed:", err)
		return
	}
	if err := service.Control(svc, "start"); err != nil {
		fmt.Println("Installed but could not start:", err)
		return
	}
	fmt.Println("--------------------------------------------------")
	fmt.Println("  TLR Time Sync installed as a Windows service.")
	fmt.Println("  It will start automatically at every boot.")
	fmt.Println("--------------------------------------------------")
	fmt.Println("")
	fmt.Println("  You can close this window.")
}

// waitForKey pauses until the user presses Enter (or 30 s elapses), so a
// console window opened by double-clicking stays visible long enough to read.
func waitForKey() {
	fmt.Println("")
	fmt.Print("Press Enter to close...")
	done := make(chan struct{})
	go func() {
		bufio.NewReader(os.Stdin).ReadString('\n')
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
}

// serviceLogWriter bridges Go's standard log package to the kardianos service
// logger so output goes to Windows Event Log / journald / syslog automatically.
type serviceLogWriter struct{ l service.Logger }

func (w *serviceLogWriter) Write(p []byte) (int, error) {
	_ = w.l.Info(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
