
.PHONY: vet
vet:
	go vet

.PHONY: coverprofie.out
coverprofile.out: vet
	(ginkgo run -v --cover --coverpkg=./,./pkg/gingk8s/ . ../mlflow-oidc-proxy/ && go tool cover -html=coverprofile.out) 2>&1 | tee test.log

