# GingK8s

Kubernetes Automation for Ginkgo tests.

# What?

GingK8s is an extention to the [Ginkgo](https://github.com/onsi/ginkgo) test framework to make building and destroying whole-cloth Kubernetes Development and Test environments easy, reliable, and repeatable.

# Why?

Tests require repeatability to be useful, and containerization allows for a far greater level of repeatability than is possible by just using whatever tools happen to be available on a developers machine or a build server. Kubernetes provides an effective way to orchestrate this, but managing Kubernetes then becomes its own challenge. GingK8s solves this challenge.

# How?

GingK8s provides types to declaratively represent common tasks in setting up a Kubernetes dev/test environment:

* Creating local KinD clusters
* Building Docker/OCI (or compatible) images
* Fetching remote images
* Loading images onto local clusters
* Creating resources from YAML Manifests
* Deploying Helm Charts
* Executing scripts within deployed containers
* Executing arbitrary go functions against deployed clusters
* Forwarding ports from containers within the cluster to the local machine
* Watching resources and events for debugging

# Examples

The [Integration tests](./gingk8s_suite_test.go) are themselves valid GingK8s tests, and thus, executable examples for you to reference.
