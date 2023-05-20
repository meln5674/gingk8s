TEST_SUITES ?= ./ ./tests/mlflow-oidc-proxy/


.PHONY: vet
vet:
	go vet

.PHONY: coverprofie.out
coverprofile.out: vet
	git -C tests/mlflow-oidc-proxy checkout go.mod
	echo "replace $$(grep gingk8s tests/mlflow-oidc-proxy/go.mod) => ../../" >> tests/mlflow-oidc-proxy/go.mod
	set -o pipefail ; (ginkgo run -v --trace --cover --coverpkg=./,./pkg/gingk8s/ $(TEST_SUITES)) 2>&1 | tee test.log
	go tool cover -html=coverprofile.out

