package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/avast/retry-go/v3"
	log "github.com/sirupsen/logrus"
)

var (
	excludePackages        string
	updatePackages         string
	severities             string
	updateIntervalDuration time.Duration

	metrics                 bool
	metricsAddr             string
	metricsPort             string
	metricsInterval         string
	metricsIntervalDuration time.Duration

	// this is used for testing
	execCommand = exec.Command
	// used by exec to avoid executing yum concurrently
	mutex = &sync.Mutex{}
)

// Default values.
const (
	defaultSeverities      string = "Important,Critical"
	defaultUpdateInterval  string = "24h"
	defaultExcludePackages string = ""
	defaultUpdatePackages  string = ""
	defaultDryRun          bool   = false

	defaultMetrics         bool   = true
	defaultMetricsAddr     string = "0.0.0.0"
	defaultMetricsPort     string = "9080"
	defaultMetricsInterval string = "1h"
)

const (
	nodeIDEnv             string = "YUMSECUPDATER_NODE_ID"
	yumPID                       = "/var/run/yum.pid"
	yumNeedUpdateExitCode int    = 100
	requireRebootExitCode int    = 1

	sentinelFile string = "/var/run/reboot-required"
)

// Config holds the general config.
type Config struct {
	dryRun          bool
	excludePackages []string
	updatePackages  []string
	severities      []string
}

func main() {
	var (
		config         = Config{}
		updateInverval string

		wg sync.WaitGroup
	)

	flag.StringVar(&excludePackages, "exclude-packages", defaultExcludePackages, "Names of packages to exclude separated with a comma")
	flag.StringVar(&updatePackages, "update-packages", defaultUpdatePackages, "Names of packages to specifically update separated with a comma, default to all")
	flag.StringVar(&severities, "severities", defaultSeverities, "Security severities to include separated with a comma, allowed values: Low,Moderate,Medium,Important,Critical")
	flag.StringVar(&updateInverval, "interval", defaultUpdateInterval, "Interval between updates")
	flag.BoolVar(&metrics, "metrics", defaultMetrics, "Enable metrics exporter")
	flag.StringVar(&metricsAddr, "metrics-addr", defaultMetricsAddr, "IP Address to expose the http metrics")
	flag.StringVar(&metricsPort, "metrics-port", defaultMetricsPort, "Port to expose the http metrics")
	flag.StringVar(&metricsInterval, "metrics-interval", defaultMetricsInterval, "Interval between metrics checks")
	flag.BoolVar(&config.dryRun, "dry-run", defaultDryRun, "Enable dry-run mode, do not run any update")
	flag.Parse()

	config.excludePackages = parseCommaSeparatedFlagValues(excludePackages)
	config.updatePackages = parseCommaSeparatedFlagValues(updatePackages)

	config.severities = parseCommaSeparatedFlagValues(severities)
	for _, s := range config.severities {
		if err := validateSeverity(s); err != nil {
			log.Fatal(err)
		}
	}

	updateIntervalDuration, err := parseDurationString(updateInverval)
	if err != nil {
		log.Fatal(err)
	}
	metricsIntervalDuration, err := parseDurationString(metricsInterval)
	if err != nil {
		log.Fatal(err)
	}

	hostname := os.Getenv(nodeIDEnv)
	if hostname == "" {
		log.Fatal("Environment variable YUMSECUPDATER_NODE_ID not found.")
	}

	sigs := make(chan os.Signal, 1)
	exitRun := make(chan struct{}, 1)
	exitMetrics := make(chan struct{}, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// handle shutdown gracefully
	wg.Add(1)
	go func() {
		sig := <-sigs
		defer wg.Done()

		log.New().WithFields(log.Fields{"signal": sig.String()}).
			Infof("graceful shutdown")

		exitRun <- struct{}{}
		if metrics {
			exitMetrics <- struct{}{}
		}

		// ensure yum can finish before exiting
		if err := ensureYumIsNotRunning(); err != nil {
			log.Error(err)
		}
	}()

	var metricsServer *MetricsServer
	if metrics {
		var err error
		metricsServer, err = newMetricsServer(hostname, metricsAddr, metricsPort)
		if err != nil {
			log.Fatalf("can not create a metrics server: %v", err)
		}
		wg.Add(1)
		go func() {
			if err := metricsServer.startServer(); err != nil {
				log.Fatal(err)
			}
		}()

		// run it once before the next timer
		metricsServer.fetchMetrics(config)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-exitMetrics:
					metricsServer.stopServer()
					wg.Done()
					return
				case <-time.After(metricsIntervalDuration):
					metricsServer.fetchMetrics(config)
				}
			}
		}()
	}

	// run it once before the next timer
	runWithRetry(config)
	if metrics {
		metricsServer.fetchMetrics(config)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-exitRun:
				return
			case <-time.After(updateIntervalDuration):
				runWithRetry(config)
				if metrics {
					metricsServer.fetchMetrics(config)
				}
			}
		}
	}()

	wg.Wait()
	log.Info("exit")
}

// ensureYumIsNotRunning is a wrapper to retry the yum check.
func ensureYumIsNotRunning() error {
	return retry.Do(
		func() error {
			if !isYumRunning() {
				return nil
			}
			log.Info("yum is currently running, waiting 10s...")
			return fmt.Errorf("yum is running")
		},
		retry.Delay(10*time.Second),
		retry.Attempts(30),
	)
}

// runWithRetry is a wrapper to retry the standard run.
func runWithRetry(config Config) {
	err := retry.Do(
		func() error {
			return run(config)
		},
		retry.Delay(30*time.Minute),
		retry.Attempts(10),
	)
	if err != nil {
		log.Error(err)
	}

	log.Infof("done, next update check on %s",
		time.Now().
			Add(updateIntervalDuration).
			Format("2006-01-02 15:04:05"))
}

// run is a wrapper that holds the logic of a standard run.
func run(config Config) error {
	updatesAvailable, err := updatesAvailable(config)
	if err != nil {
		return err
	}

	if config.dryRun {
		log.Info("dry-run mode enabled, do not update")
		return nil
	}

	if updatesAvailable {
		if err := runUpdates(config); err != nil {
			return err
		}
	}

	// Even if no updates are availabe, server may still
	// need to be rebooted.
	rebootRequired, err := requireReboot()
	if err != nil {
		return err
	}
	if !rebootRequired {
		return nil
	}

	// create sentinel file for kured
	if err := createSentinelFile(); err != nil {
		return err
	}

	return nil
}

// defaultYumCommand returns the default yum command.
func defaultYumCommand() []string {
	return []string{"yum", "-y", "-q"}
}

// buildYumUpdatesCommand returns the exec command that is used
// to check and update packages with yum.
func buildYumUpdatesCommand(action string, config Config) *exec.Cmd {
	cmd := defaultYumCommand()
	cmd = append(cmd, action, "--security")

	for _, pkg := range config.excludePackages {
		cmd = append(cmd, "--exclude="+pkg)
	}
	for _, severity := range config.severities {
		cmd = append(cmd, "--sec-severity="+severity)
	}

	cmd = append(cmd, config.updatePackages...)
	cmd = buildHostCommand(cmd)

	return newCommand(cmd)
}

// updatesAvailable checks if some updates are available.
func updatesAvailable(config Config) (bool, error) {
	log.Infof("check if updates are available")

	cmd := buildYumUpdatesCommand("check-update", config)
	if err := runCommand(cmd); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			if exitCode == yumNeedUpdateExitCode {
				log.Infof("updates available")
				return true, nil
			}
		}
		return false, fmt.Errorf("yum-check-update did not run successfully: %v", err)
	}

	log.Infof("no updates available")

	return false, nil
}

// runUpdates starts the update of packages.
func runUpdates(config Config) error {
	log.Infof("update security packages")

	cmd := buildYumUpdatesCommand("update", config)
	if err := runCommand(cmd); err != nil {
		return err
	}

	log.Infof("yum-update ran successfully")

	return nil
}

// buildRequireRebootCommand returns the exec command to
// check if a reboot is required.
func buildRequireRebootCommand() *exec.Cmd {
	cmd := []string{"needs-restarting", "-r"}
	cmd = buildHostCommand(cmd)
	return newCommand(cmd)
}

// requireReboot checks if a reboot is required.
func requireReboot() (bool, error) {
	log.Infof("check if reboot is required")

	cmd := buildRequireRebootCommand()
	if err := runCommand(cmd); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			if exitCode == requireRebootExitCode {
				log.Infof("reboot required")
				return true, nil
			}
		}
		return false, fmt.Errorf("needs-restarting did not run successfully: %v", err)
	}

	log.Infof("no reboot required")

	return false, nil
}

// buildCreateSentinelFileCommand returns the exec command to
// create the kured sentinel file.
func buildCreateSentinelFileCommand() *exec.Cmd {
	cmd := []string{"touch", sentinelFile}
	cmd = buildHostCommand(cmd)
	return newCommand(cmd)
}

// createSentinelFile creates the kured sentinel file.
func createSentinelFile() error {
	log.Infof("create sentinel file")

	cmd := buildCreateSentinelFileCommand()
	if err := runCommand(cmd); err != nil {
		return fmt.Errorf("create sentinel failed: %v", err)
	}

	log.Infof("sentinel file created successfully")

	return nil
}

var varYumPID = yumPID

// isYumRunning checks if yum is currently running.
func isYumRunning() bool {
	if _, err := os.Stat(varYumPID); err == nil {
		return true
	}

	return false
}

// newCommand creates a new Command with stdout/stderr wired to the standard logger.
func newCommand(command []string) *exec.Cmd {
	name := command[0]
	args := command[1:]

	cmd := execCommand(name, args...)
	cmd.Stdout = log.NewEntry(log.StandardLogger()).
		WithField("cmd", cmd.Args[0]).
		WithField("std", "out").
		WriterLevel(log.InfoLevel)

	cmd.Stderr = log.NewEntry(log.StandardLogger()).
		WithField("cmd", cmd.Args[0]).
		WithField("std", "err").
		WriterLevel(log.WarnLevel)

	return cmd
}

// buildHostCommand writes a new command to run in the host namespace
func buildHostCommand(command []string) []string {
	// From the container, we nsenter into the proper PID to run the hostCommand.
	// For this, daemonset need to be configured with hostPID:true and privileged:true
	cmd := []string{"/usr/bin/nsenter", "-m/proc/1/ns/mnt", "--"}
	cmd = append(cmd, command...)

	return cmd
}

// runCommand runs a command and waits for it to complete.
func runCommand(cmd *exec.Cmd) error {
	log.Infof("running command: %v", cmd.Args)
	mutex.Lock()
	err := cmd.Run()
	mutex.Unlock()

	return err
}
