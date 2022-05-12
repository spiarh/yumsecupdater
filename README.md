# yumsecupdater

**Note: I built this tool for OpenShift 3.11 clusters but they are all
decommissioned now so this tool is not used anymore.**

`yumsecupdater` is a tool to keep an openshift/kubernetes node running
on rhel 7 (only version tested) up-to-date. It has to be deployed
as a `DaemonSet` and provides prometheus metrics.

It is also meant to be deployed along with [kured](https://github.com/weaveworks/kured)
as it will create the sentinel file (/var/run/reboot-required) automatically
if a reboot is required.

See [./manifests](./manifests) for the deployment.

:warning: it is not recommended to expose to the world the list
of outdated packages, consider using network policies to restrain
the access only from prometheus. Adding TLS client validation to the
tool would be welome as well.

## Metrics

It exports two types of metrics:


* yumsecupdater_packages_with_update_total

This metrics exports the total number of packages with security updates.

> yumsecupdater_packages_with_update_total{node="localhost"} 90


* yumsecupdater_package_with_update

This metrics exports a package with security udate.

> yumsecupdater_package_with_update{arch="noarch",name="NetworkManager-config-server",node="localhost",repo="rhel-7-server-rpms",version="1:1.18.8-2.el7_9"} 1
> yumsecupdater_package_with_update{arch="noarch",name="bind-license",node="localhost",repo="rhel-7-server-rpms",version="32:9.11.4-26.P2.el7_9.5"} 1
> yumsecupdater_package_with_update{arch="noarch",name="elfutils-default-yama-scope",node="localhost",repo="rhel-7-server-rpms",version="0.176-5.el7"} 1
> yumsecupdater_package_with_update{arch="noarch",name="emacs-filesystem",node="localhost",repo="rhel-7-server-rpms",version="1:24.3-23.el7"} 1
> yumsecupdater_package_with_update{arch="noarch",name="grub2-common",node="localhost",repo="rhel-7-server-rpms",version="1:2.02-0.87.el7_9.6"} 1


## Usage

```
Usage of ./yumsecupdater:
  -dry-run
    	Enable dry-run mode, do not run any update
  -exclude-packages string
    	Names of packages to exclude separated with a comma
  -interval string
    	Interval between updates (default "24h")
  -metrics
    	Enable metrics exporter (default true)
  -metrics-addr string
    	IP Address to expose the http metrics (default "0.0.0.0")
  -metrics-interval string
    	Interval between metrics checks (default "12h")
  -metrics-port string
    	Port to expose the http metrics (default "9080")
  -severities string
    	Security severities to include separated with a comma, allowed values: Low,Moderate,Medium,Important,Critical (default "Important,Critical")
  -update-packages string
    	Names of packages to specifically update separated with a comma, default to all
```
