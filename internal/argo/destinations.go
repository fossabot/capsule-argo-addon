package argo

import (
	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

// Verify if the project already has the destination
func ProjectHasDestination(appProject *argocdv1alpha1.AppProject, dest argocdv1alpha1.ApplicationDestination) bool {
	// Check if the destination already exists
	exists := false
	for _, e := range appProject.Spec.Destinations {
		if e.Name == dest.Name && e.Namespace == dest.Namespace && e.Server == dest.Server {
			exists = true
			break
		}
	}

	return exists
}

// Remove a destination from the project
func RemoveProjectDestination(appProject *argocdv1alpha1.AppProject, dest argocdv1alpha1.ApplicationDestination) {
	newDestinations := []argocdv1alpha1.ApplicationDestination{}
	for _, e := range appProject.Spec.Destinations {
		if !(e.Name == dest.Name && e.Namespace == dest.Namespace && e.Server == dest.Server) {
			newDestinations = append(newDestinations, e)
		}
	}
	appProject.Spec.Destinations = newDestinations
}
