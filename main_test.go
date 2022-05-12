package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	testName    string
	hostCommand string = "/usr/bin/nsenter -m/proc/1/ns/mnt -- "
)

func helperCommand(command string, args ...string) *exec.Cmd {
	cs := []string{testName, "-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	return cmd
}

const (
	testDefaultSuccess = "default-success"
	testDefaultFailure = "default-failure"

	testUpdateAvailable   = "update-available"
	testNoUpdateAvailable = "no-update-available"

	testRebootRequired   = "reboot-required"
	testNoRebootRequired = "no-reboot-required"

	testFailUpdateAvailable = "fail-update-available"
	testFailRebootRequired  = "fail-reboot-required"

	testRunUpdateAvailable = "run-update-available"

	testMetricsUpdateAvailable = "metrics-update-available"
)

var exitCodes = map[string]int{
	testDefaultSuccess:      0,
	testDefaultFailure:      10,
	testNoUpdateAvailable:   0,
	testUpdateAvailable:     100,
	testNoRebootRequired:    0,
	testRebootRequired:      1,
	testFailUpdateAvailable: 1,
	testFailRebootRequired:  10,
}

// TestHelperProcess isn't a real test. It's used as a helper process
// to mock running the commands.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args

	// get testName from the first arg so we can use that
	// in the switch below to define the ouputs/exit-codes.
	testName := args[1]
	args = append(args[:1], args[1+1:]...)

	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "No command\n")
		os.Exit(2)
	}

	switch testName {
	case testNoUpdateAvailable, testNoRebootRequired:
		os.Exit(exitCodes[testNoUpdateAvailable])
	case testRebootRequired, testFailUpdateAvailable:
		os.Exit(exitCodes[testRebootRequired])
	case testUpdateAvailable:
		os.Exit(exitCodes[testUpdateAvailable])
	case testFailRebootRequired:
		os.Exit(exitCodes[testFailRebootRequired])
	case testRunUpdateAvailable, testMetricsUpdateAvailable:
		lenDefaultCommand := len(strings.Split(hostCommand, " ")) - 1
		command := args[lenDefaultCommand]
		if command == "needs-restarting" {
			os.Exit(exitCodes[testRebootRequired])
		}
		if command == "touch" {
			os.Exit(exitCodes[testDefaultSuccess])
		}
		if command == "yum" {
			action := args[lenDefaultCommand+len(defaultYumCommand())]
			if action == "check-update" {
				// write outputs to generate metrics
				pkgsWithUpdates := strings.Join(validUpdatesAvailable, "\n")
				fmt.Fprintf(os.Stdout, pkgsWithUpdates)
				os.Exit(exitCodes[testUpdateAvailable])
			}
			if action == "update" {
				os.Exit(exitCodes[testDefaultSuccess])
			}
		}
	// failed
	default:
		os.Exit(exitCodes[testDefaultFailure])
	}
}

func TestBuildYumCommand(t *testing.T) {
	var tests = []struct {
		config      Config
		action      string
		expectedCmd string
	}{
		{
			Config{
				excludePackages: []string{"atomic*", "etcd"},
				severities:      []string{"Important", "Critical"},
			},
			"check-update",
			"yum -y -q check-update --security --exclude=atomic* --exclude=etcd --sec-severity=Important --sec-severity=Critical",
		},
		{
			Config{
				updatePackages: []string{"sudo", "openssl"},
				severities:     []string{"Important"},
			},
			"update",
			"yum -y -q update --security --sec-severity=Important sudo openssl",
		},
	}

	for _, tt := range tests {
		cmd := buildYumUpdatesCommand(tt.action, tt.config)
		if strings.Join(cmd.Args, " ") != hostCommand+tt.expectedCmd {
			t.Fatal(cmd.Args, tt.expectedCmd)
		}
	}
}

func TestBuildCommands(t *testing.T) {
	var tests = []struct {
		function    func() *exec.Cmd
		expectedCmd string
	}{
		{
			buildCreateSentinelFileCommand,
			"touch /var/run/reboot-required",
		},
		{
			buildRequireRebootCommand,
			"needs-restarting -r",
		},
	}
	for _, tt := range tests {
		cmd := tt.function()
		if strings.Join(cmd.Args, " ") != hostCommand+tt.expectedCmd {
			t.Fatal(cmd.Args, tt.expectedCmd)
		}
	}
}

func TestUpdatesAvailable(t *testing.T) {
	var tests = []struct {
		testName  string
		available bool
		wantErr   bool
	}{
		{testUpdateAvailable, true, false},
		{testNoUpdateAvailable, false, false},
		{testFailUpdateAvailable, false, true},
	}

	for _, tt := range tests {
		testName = tt.testName
		execCommand = helperCommand
		defer func() { execCommand = exec.Command }()

		available, err := updatesAvailable(Config{})
		if !tt.wantErr && err != nil {
			t.Fatal(err)
		}

		if available != tt.available {
			t.Fatal(available)
		}
	}
}

func TestRequireReboot(t *testing.T) {
	var tests = []struct {
		testName string
		required bool
		wantErr  bool
	}{
		{testRebootRequired, true, false},
		{testNoRebootRequired, false, false},
		{testFailRebootRequired, false, true},
	}

	for _, tt := range tests {
		testName = tt.testName
		execCommand = helperCommand
		defer func() { execCommand = exec.Command }()

		required, err := requireReboot()
		if !tt.wantErr && err != nil {
			t.Fatal(err)
		}

		if required != tt.required {
			t.Fatal(required)
		}
	}
}

func TestIsYumRunning(t *testing.T) {
	defer func() {
		varYumPID = yumPID
	}()

	varYumPID = "main.go"
	result := isYumRunning()
	assert.True(t, result)

	varYumPID = "donotexist"
	result = isYumRunning()
	assert.False(t, result)
}

func TestRun(t *testing.T) {
	testName = testRunUpdateAvailable
	execCommand = helperCommand
	defer func() { execCommand = exec.Command }()

	runWithRetry(Config{dryRun: true})
	if err := run(Config{}); err != nil {
		t.Fatal(err)
	}

	if err := run(Config{dryRun: true}); err != nil {
		t.Fatal(err)
	}
}
