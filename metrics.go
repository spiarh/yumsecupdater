package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

const metricsPath = "/metrics"

// MetricsServer holds the http server with the metrics.
type MetricsServer struct {
	*http.Server
	hostname string

	pkgsWithUpdateTotal *prometheus.GaugeVec
	pkgWithUpdate       *prometheus.CounterVec
}

type packageWithUpdate struct {
	name, arch, version, repo string
}

// Prometheus metrics.
var ()

func newPkgsWithUpdateTotalGauge() *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "yumsecupdater_packages_with_update_total",
		Help: "Total packages with security updates.",
	},
		[]string{"node"},
	)
}

func newPkgWithUpdateCounter() *prometheus.CounterVec {
	return prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "yumsecupdater_package_with_update",
			Help: "Package with security update.",
		},
		[]string{"node", "name", "arch", "version", "repo"},
	)
}

// newMetricsServer returns a metricsServer to manage metrics.
func newMetricsServer(hostname, addr, port string) (*MetricsServer, error) {
	pkgsWithUpdateTotal := newPkgsWithUpdateTotalGauge()
	pkgWithUpdate := newPkgWithUpdateCounter()

	prometheus.MustRegister(pkgsWithUpdateTotal)
	prometheus.MustRegister(pkgWithUpdate)

	r := mux.NewRouter()
	r.Handle(metricsPath, promhttp.Handler())

	return &MetricsServer{
		Server: &http.Server{
			Addr:    fmt.Sprintf("%s:%s", addr, port),
			Handler: r,
		},
		pkgsWithUpdateTotal: pkgsWithUpdateTotal,
		pkgWithUpdate:       pkgWithUpdate,
		hostname:            hostname,
	}, nil
}

func (m *MetricsServer) startServer() error {
	log.WithFields(log.Fields{"addr": m.Addr}).
		Infof("start metrics server")
	if err := m.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (m *MetricsServer) stopServer() {
	log.WithFields(log.Fields{"addr": m.Addr}).
		Infof("stop metrics server")
	m.Shutdown(context.TODO())
}

func (m *MetricsServer) fetchMetrics(config Config) {
	packagesWithUpdates, err := metricsUpdatesAvailable(config)
	if err != nil {
		log.Error(err)
	}
	m.setMetrics(packagesWithUpdates)
}
func (m *MetricsServer) setMetrics(pkgs []packageWithUpdate) {
	m.setPkgsWithUpdateTotal(pkgs)
	m.setPkgWithUpdate(pkgs)
}

func (m *MetricsServer) setPkgWithUpdate(packagesWithUpdates []packageWithUpdate) {
	// clean up the current label first to remove metrics from updated packages.
	m.pkgWithUpdate.Reset()
	for _, pkg := range packagesWithUpdates {
		labels := promLabelsFromPackageWithUpdate(m.hostname, pkg)
		m.pkgWithUpdate.With(labels).Add(1)
	}
}

func (m *MetricsServer) setPkgsWithUpdateTotal(pkgs []packageWithUpdate) {
	m.pkgsWithUpdateTotal.With(prometheus.Labels{"node": m.hostname}).
		Set(float64(len(pkgs)))
}

var packageWithUpdateRegex = regexp.MustCompile(`^([\w\-\_]*)\.([\w\-\_]+)\s+(\d+[\w\.\-\_\:]+)\s+([\w\.\-\_]+)\s*$`)

func parseUpdatesAvailable(output []byte) ([]packageWithUpdate, error) {
	sc := bufio.NewScanner(bytes.NewReader(output))
	packages := make([]packageWithUpdate, 0)

	for sc.Scan() {
		line := sc.Bytes()

		if packageWithUpdateRegex.Match(line) {
			// safeguard in case the regex matches wrong
			pkgSlice := strings.Fields(string(line))
			if len(pkgSlice) != 3 {
				return packages, fmt.Errorf("invalid parsed slice: %v", pkgSlice)
			}

			// parse name and arch from format name.arch
			pkgNameArch := strings.Split(pkgSlice[0], ".")

			pkg := packageWithUpdate{
				name:    pkgNameArch[0],
				arch:    pkgNameArch[1],
				version: pkgSlice[1],
				repo:    pkgSlice[2],
			}

			packages = append(packages, pkg)
		}
	}

	return packages, nil
}

// metricsupdatesAvailable checks if some updates are available to generate metrics.
func metricsUpdatesAvailable(config Config) ([]packageWithUpdate, error) {
	log.WithField("component", "metrics").
		Infof("check if updates are available")

	result := bytes.Buffer{}
	cmd := buildYumUpdatesCommand("check-update", config)
	cmd.Stderr = &result
	cmd.Stdout = &result

	pkgs := make([]packageWithUpdate, 0)

	if err := runCommand(cmd); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			if exitCode == yumNeedUpdateExitCode {
				log.WithField("component", "metrics").
					Infof("updates available")
				return parseUpdatesAvailable(result.Bytes())
			}
		} else {
			return pkgs, fmt.Errorf("yum-check-update did not run successfully: %v", err)
		}
	}

	return pkgs, nil
}

func promLabelsFromPackageWithUpdate(hostname string, pkg packageWithUpdate) prometheus.Labels {
	return prometheus.Labels{
		"node":    hostname,
		"name":    pkg.name,
		"arch":    pkg.arch,
		"version": pkg.version,
		"repo":    pkg.repo,
	}
}
