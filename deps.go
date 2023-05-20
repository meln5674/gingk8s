package gingk8s

type ResourceDependencies struct {
	ThirdPartyImages []ThirdPartyImageID
	CustomImages     []CustomImageID
	Manifests        []ManifestsID
	Releases         []ReleaseID
	ClusterActions   []ClusterActionID
}

var NoDependencies = ResourceDependencies{}

func (r *ResourceDependencies) allIDs(clusterID string) []string {
	dependsOn := []string{}
	for _, image := range r.ThirdPartyImages {
		dependsOn = append(dependsOn, state.clusterThirdPartyLoads[clusterID][image.id])
	}
	for _, image := range r.CustomImages {
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
