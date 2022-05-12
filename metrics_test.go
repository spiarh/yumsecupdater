package main

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

var validUpdatesAvailable []string = []string{
	"117.x86_64                   1:1.0.2k-21.el7_9              rhel-7-server-rpms",
	"118.x86_64                   1:1.0.2k-21.el7_9              rhel-7-server-rpms      ",
	"pkg-noarch.noarch            32:9.11.4-26.P2.el7_9.5        rhel-7-server_rpms",
	"pkg-x86_64.x86_64            2:1.13.1-206.git7d71120.el7_9  rhel-7-server.extras-rpms",
	"pkg-with_spec-chars.x86_64   1.8.23-10.el7_9.1              rhel-7-server-rpms",
}

var invalidUpdatesAvailable []string = []string{
	"invalid-repo.noarch          32:9.11.4-26.P2.el7_9.5    rhel-7-se$rver-rpms",
	"valid.noarch                 32:9.11.4-26.P2.el7_9.5    rhel-7-server-rpms 3edfwefwe3",
	"in.valid.noarch              32:9.11.4-26.P2.el7_9.5    rhel-7-server-rpms 3edfwefwe3",
	"pkg-letter-start-ver.x86_64  d1.8.23-10.el7_9.1         rhel-7-server-rpms",
	"pkg-ver-special-char.x86_64  1.8.23-10@.el7_9.1         rhel-7-server-rpms",
	"pkg-no-repo.x86_64           4.8.23-10.el7_9.1",
	"pkg-no-repo2.x86_64          2.8.23-10.el7_9.1    ",
	"pkg-no-arch                  1.8.23-10.el7_9.1          rhel-7-server-rpms",
	"",
	"   ",
}

func TestParseUpdatesAvailable(t *testing.T) {
	for _, tt := range validUpdatesAvailable {
		result, err := parseUpdatesAvailable([]byte(tt))
		if len(result) != 1 {
			t.Fatalf("line=%s, result=%d, expected%d", tt, len(result), 1)
		}
		assert.NoError(t, err)
	}

	for _, tt := range invalidUpdatesAvailable {
		result, err := parseUpdatesAvailable([]byte(tt))
		if len(result) != 0 {
			t.Fatalf("line=%s, result=%d, expected%d", tt, len(result), 0)
		}
		assert.NoError(t, err)
	}

	data, err := ioutil.ReadFile("./testdata/package-with-updates_full_90_pkgs")
	assert.NoError(t, err)

	result, err := parseUpdatesAvailable(data)
	if len(result) != 90 {
		t.Fatalf("result=%d, expected%d", len(result), 90)
	}
	assert.NoError(t, err)
}

func testMetrics(t *testing.T, pkgs []packageWithUpdate, assertFunc func(*testing.T, string, string), output string) {
	m, err := newMetricsServer("localhost", "localhost", "9080")
	assert.NoError(t, err)

	defer prometheus.Unregister(m.pkgsWithUpdateTotal)
	defer prometheus.Unregister(m.pkgWithUpdate)

	m.setMetrics(pkgs)

	req, err := http.NewRequest("GET", "/metrics", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	m.Server.Handler.ServeHTTP(rr, req)
	assert.Equal(t, rr.Code, http.StatusOK)

	body := rr.Body.String()
	log.Println(body)
	assertFunc(t, body, output)
}

func TestMetricsFromFullList(t *testing.T) {
	expectedOutput, err := ioutil.ReadFile("./testdata/package-with-updates_full_90_pkgs_metrics")
	assert.NoError(t, err)

	data, err := ioutil.ReadFile("./testdata/package-with-updates_full_90_pkgs")
	assert.NoError(t, err)

	result, err := parseUpdatesAvailable(data)
	assert.NoError(t, err)
	testMetrics(t, result, assertMetricsOutput, string(expectedOutput))
}

func TestMetricsNoPkgs(t *testing.T) {
	expectedOutput := `# HELP yumsecupdater_packages_with_update_total Total packages with security updates.
# TYPE yumsecupdater_packages_with_update_total gauge
yumsecupdater_packages_with_update_total{node="localhost"} 0`

	notExpectedOutput := "yumsecupdater_package_with_update"
	data := strings.Join(invalidUpdatesAvailable, "\n")

	result, err := parseUpdatesAvailable([]byte(data))
	assert.NoError(t, err)
	testMetrics(t, result, assertMetricsOutput, expectedOutput)
	testMetrics(t, result, assertMetricsNotInOutput, notExpectedOutput)
}

func TestFetchMetricsE2E(t *testing.T) {
	expectedOutput := `# HELP yumsecupdater_package_with_update Package with security update.
# TYPE yumsecupdater_package_with_update counter
yumsecupdater_package_with_update{arch="noarch",name="pkg-noarch",node="localhost",repo="rhel-7-server_rpms",version="32:9.11.4-26.P2.el7_9.5"} 1
yumsecupdater_package_with_update{arch="x86_64",name="117",node="localhost",repo="rhel-7-server-rpms",version="1:1.0.2k-21.el7_9"} 1
yumsecupdater_package_with_update{arch="x86_64",name="118",node="localhost",repo="rhel-7-server-rpms",version="1:1.0.2k-21.el7_9"} 1
yumsecupdater_package_with_update{arch="x86_64",name="pkg-with_spec-chars",node="localhost",repo="rhel-7-server-rpms",version="1.8.23-10.el7_9.1"} 1
yumsecupdater_package_with_update{arch="x86_64",name="pkg-x86_64",node="localhost",repo="rhel-7-server.extras-rpms",version="2:1.13.1-206.git7d71120.el7_9"} 1
# HELP yumsecupdater_packages_with_update_total Total packages with security updates.
# TYPE yumsecupdater_packages_with_update_total gauge
yumsecupdater_packages_with_update_total{node="localhost"} 5`

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	m, err := newMetricsServer("localhost", "localhost", "9080")
	assert.NoError(t, err)

	go m.startServer()

	testName = testMetricsUpdateAvailable
	execCommand = helperCommand
	defer func() { execCommand = exec.Command }()

	m.fetchMetrics(Config{})
	req, err := http.NewRequest("GET", "http://localhost:9080/metrics", nil)
	assert.NoError(t, err)

	rr, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, rr.StatusCode, http.StatusOK)

	body, err := ioutil.ReadAll(rr.Body)
	assert.NoError(t, err)
	log.Println(string(body))
	assertMetricsOutput(t, string(body), expectedOutput)

	<-ctx.Done()
	m.stopServer()
}

func assertMetricsOutput(t *testing.T, body, expectedOutput string) {
	for _, line := range strings.Split(expectedOutput, "\n") {
		if line == "" {
			continue
		}
		assert.Contains(t, body, line)
	}
}

func assertMetricsNotInOutput(t *testing.T, body, notExpectedOutput string) {
	for _, line := range strings.Split(notExpectedOutput, "\n") {
		if line == "" {
			continue
		}
		assert.NotContains(t, body, line)
	}
}
