package gingk8s_test

import (
	"context"
	"testing"

	. "github.com/meln5674/gingk8s"
	gingk8s "github.com/meln5674/gingk8s/pkg/gingk8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGingk8s(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gingk8s Suite")
}

var _ = BeforeSuite(func(ctx context.Context) {
	// Gingk8sOptions sets global options for the suite
	Gingk8sOptions(gingk8s.SuiteOpts{
		// If you want to leave your suite resources running after the suite finishes,
		// uncomment this line. You'll need to manually delete the resulting cluster.
		// NoSuiteCleanup: true,
	})

	// CustomImage and CustomImages register custom images that must be built as part of the
	// suite
	customImageIDs := CustomImages(&customWordpressImage)
	// ThirdPartyImage and ThirdPartyImages register pre-built images that (by default),
	// must be pulled from a removte source
	thirdPartyImageIDs := ThirdPartyImages(&mariadbImage)

	// Cluster registers a cluster that must be created, as well as all images that must be loaded
	// onto it
	clusterID := Cluster(&cluster, ClusterDependencies{
		CustomImages:     customImageIDs,
		ThirdPartyImages: thirdPartyImageIDs,
	})

	// Release, Manifests, and ClusterAction register steps to be taken once a cluster is ready,
	// as well as any actions that must succeed, such as waiting for a set of images to finish
	// loading
	Release(clusterID, &myWordpress, ResourceDependencies{
		CustomImages:     customImageIDs,
		ThirdPartyImages: thirdPartyImageIDs,
	})

	// Gingk8sSetup is the entrypoint to Gingk8s, call it once your suite is fully registered to
	// build your environment to prepare for your tests
	Gingk8sSetup(ctx)
})

var (
	cluster = gingk8s.KindCluster{
		Name:    "gingk8s",
		TempDir: "./tmp",
	}

	bitnami = gingk8s.HelmRepo{
		Name: "bitnami",
		URL:  "https://charts.bitnami.com/bitnami",
	}

	mariadbImage = gingk8s.ThirdPartyImage{
		Name: "docker.io/bitnami/mariadb:10.6.12-debian-11-r16",
	}

	customWordpressImage = gingk8s.CustomImage{
		Registry:   "registry.local",
		Repository: "gingk8s/my-custom-wordpress",
		BuildArgs: map[string]string{
			"BASE_IMAGE": "docker.io/bitnami/wordpress:6.2.0-debian-11-r8",
		},
		ContextDir: "hack/custom-wordpress-image",
	}

	wordpress = gingk8s.HelmChart{
		RemoteChartInfo: gingk8s.RemoteChartInfo{
			Repo:    &bitnami,
			Name:    "wordpress",
			Version: "15.4.0",
		},
	}
	myWordpress = gingk8s.HelmRelease{
		Name:  "wordpress",
		Chart: &wordpress,
		Set: map[string]interface{}{
			"ingress.enabled":  true,
			"service.type":     "ClusterIP",
			"image.registry":   customWordpressImage.Registry,
			"image.repository": customWordpressImage.Repository,
			"image.tag":        gingk8s.DefaultCustomImageTag,

			"image.pullPolicy":         "Never",
			"mariadb.image.pullPolicy": "Never",
		},
		UpgradeFlags: []string{"--debug"},
	}
)
