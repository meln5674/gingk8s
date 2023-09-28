package gingk8s

import "fmt"

type ResourceDependencies struct {
	ThirdPartyImages []ThirdPartyImageID
	CustomImages     []CustomImageID
	Manifests        []ManifestsID
	Releases         []ReleaseID
	ClusterActions   []ClusterActionID
}

type ClusterDependencies struct {
	ThirdPartyImages []ThirdPartyImageID
	CustomImages     []CustomImageID
}

var NoDependencies = ResourceDependencies{}

type ResourceDependency interface {
	AddResourceDependency(*ResourceDependencies)
}

type ClusterDependency interface {
	AddClusterDependency(*ClusterDependencies)
}

func (r *ResourceDependencies) AddResourceDependency(dep *ResourceDependencies) {
	dep.ThirdPartyImages = append(dep.ThirdPartyImages, r.ThirdPartyImages...)
	dep.CustomImages = append(dep.CustomImages, r.CustomImages...)
	dep.Manifests = append(dep.Manifests, r.Manifests...)
	dep.Releases = append(dep.Releases, r.Releases...)
	dep.CustomImages = append(dep.CustomImages, r.CustomImages...)
}

func (c ClusterDependencies) AddClusterDependency(dep *ClusterDependencies) {
	dep.ThirdPartyImages = append(dep.ThirdPartyImages, c.ThirdPartyImages...)
	dep.CustomImages = append(dep.CustomImages, c.CustomImages...)
}

func forResourceDependencies(deps ...ResourceDependency) *ResourceDependencies {
	allDeps := ResourceDependencies{}
	for _, dep := range deps {
		dep.AddResourceDependency(&allDeps)
	}

	return &allDeps
}

func forClusterDependencies(deps ...ClusterDependency) *ClusterDependencies {
	allDeps := ClusterDependencies{}
	for _, dep := range deps {
		dep.AddClusterDependency(&allDeps)
	}

	return &allDeps
}

func (r *ResourceDependencies) allIDs(clusterID string) []string {
	dependsOn := []string{}
	for _, image := range r.ThirdPartyImages {
		if _, ok := state.clusterThirdPartyLoads[clusterID]; !ok {
			panic(fmt.Sprintf("BUG: No cluster with ID %s", clusterID))
		}
		if _, ok := state.clusterThirdPartyLoads[clusterID][image.id]; !ok {
			panic(fmt.Sprintf("BUG: Third-Party image %s is not set to load to cluster %s", image.id, clusterID))
		}
		dependsOn = append(dependsOn, state.clusterThirdPartyLoads[clusterID][image.id])
	}
	for _, image := range r.CustomImages {
		if _, ok := state.clusterCustomLoads[clusterID]; !ok {
			panic(fmt.Sprintf("BUG: No cluster with ID %s", clusterID))
		}
		if _, ok := state.clusterCustomLoads[clusterID][image.id]; !ok {
			panic(fmt.Sprintf("BUG: Custom image %s is not set to load to cluster %s", image.id, clusterID))
		}
		dependsOn = append(dependsOn, state.clusterCustomLoads[clusterID][image.id])
	}
	for _, manifests := range r.Manifests {
		dependsOn = append(dependsOn, manifests.id)
	}
	for _, release := range r.Releases {
		dependsOn = append(dependsOn, release.id)
	}
	for _, action := range r.ClusterActions {
		dependsOn = append(dependsOn, action.id)
	}
	return dependsOn
}
