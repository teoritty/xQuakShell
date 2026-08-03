#!/usr/bin/env bash
set -euo pipefail

DOMAIN_COVERPKG="xquakshell/internal/domain/plugin"
INFRA_COVERPKG="xquakshell/internal/infra/plugin,xquakshell/internal/infra/plugin/assets,xquakshell/internal/infra/plugin/bundle,xquakshell/internal/infra/plugin/capability,xquakshell/internal/infra/plugin/ipc,xquakshell/internal/infra/plugin/lifecycle"
USECASE_COVERPKG="xquakshell/internal/usecase"
USECASE_MIN="${USECASE_MIN:-50}"

DOMAIN_MIN="${DOMAIN_MIN:-80}"
INFRA_MIN="${INFRA_MIN:-60}"

go test ./internal/domain/plugin/ ./test/unit/plugin/ \
  -coverprofile=cov-domain.out -covermode=atomic \
  -coverpkg="${DOMAIN_COVERPKG}"

domain_pct="$(go tool cover -func=cov-domain.out | awk '/total:/ {gsub(/%/,"",$3); print $3}')"
echo "domain/plugin coverage: ${domain_pct}% (min ${DOMAIN_MIN}%)"
awk "BEGIN {exit !(${domain_pct} >= ${DOMAIN_MIN})}"

go test ./internal/infra/plugin/ ./internal/infra/plugin/ipc/ ./test/unit/plugin/ \
  -coverprofile=cov-infra.out -covermode=atomic \
  -coverpkg="${INFRA_COVERPKG}"

infra_pct="$(go tool cover -func=cov-infra.out | awk '/total:/ {gsub(/%/,"",$3); print $3}')"
echo "infra/plugin coverage: ${infra_pct}% (min ${INFRA_MIN}%)"
awk "BEGIN {exit !(${infra_pct} >= ${INFRA_MIN})}"

go test ./internal/usecase/ ./test/unit/plugin/ \
  -coverprofile=cov-usecase.out -covermode=atomic \
  -coverpkg="${USECASE_COVERPKG}"

usecase_pct="$(go tool cover -func=cov-usecase.out | awk '/total:/ {gsub(/%/,"",$3); print $3}')"
echo "usecase plugin coverage: ${usecase_pct}% (min ${USECASE_MIN}%)"
awk "BEGIN {exit !(${usecase_pct} >= ${USECASE_MIN})}"
