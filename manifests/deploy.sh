#!/usr/bin/env bash

DIR=$(cd "$( dirname "$0" )" && pwd)
DEPLOY_FILE="$DIR/deploy.yaml"
DRY_RUN="${1:-false}"

if [[ "$1" == "" ]]; then
  echo "Usage: $0 [dry-run]"
  echo "* dry-run: true or false, defines if yumsecupdater should actually updates packages"
  echo
  echo "Example: $0 true"
  exit 1
fi

set -euo pipefail

echo -e ">>> yumsecupdater dry-run: $DRY_RUN"

GRAFANA_ROUTE_HOST=$(oc -n openshift-monitoring get route grafana -ojsonpath='{.spec.host}' | sed 's/grafana/grafana-nodes-updates/g')

sed -i "s/host: REPLACE_HOST.*/host: $GRAFANA_ROUTE_HOST/" "$DEPLOY_FILE"
sed -i "s/dry-run=.*/dry-run=$DRY_RUN/" "$DEPLOY_FILE"
oc apply -f "$DEPLOY_FILE"

echo -e "\n>>> Grafana dashboard: https://$GRAFANA_ROUTE_HOST"
