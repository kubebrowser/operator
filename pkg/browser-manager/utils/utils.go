package utils

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	BrowserPortNumber = 3000
	VNCPortNumber     = 5900

	DeploymentStatusReady       = "Ready"
	DeploymentStatusProgressing = "Progressing"
	DeploymentStatusStopped     = "Stopped"
)

func AreResourcesEqual(r1 corev1.ResourceRequirements, r2 corev1.ResourceRequirements) bool {
	if !areResourceListsEqual(r1.Requests, r2.Requests) {
		return false
	}

	if !areResourceListsEqual(r1.Limits, r2.Limits) {
		return false
	}

	return true
}

func areResourceListsEqual(rl1 corev1.ResourceList, rl2 corev1.ResourceList) bool {
	if !areEqual(rl1.Cpu(), rl2.Cpu()) {
		return false
	}
	if !areEqual(rl1.Memory(), rl2.Memory()) {
		return false
	}
	if !areEqual(rl1.StorageEphemeral(), rl2.StorageEphemeral()) {
		return false
	}

	return true
}

func areEqual(a *resource.Quantity, b *resource.Quantity) bool {
	if a == nil && b != nil {
		return false
	}

	if b == nil && a != nil {
		return false
	}

	if a != nil && !a.Equal(*b) {
		return false
	}

	return true
}

func GetDeploymentStatus(dep *appsv1.Deployment) string {
	desiredReplicas := int32(1)

	if dep.Spec.Replicas != nil {
		desiredReplicas = *dep.Spec.Replicas
	}

	if desiredReplicas != 0 && desiredReplicas == dep.Status.ReadyReplicas {
		return DeploymentStatusReady
	}

	terminatingPods := int32(0)
	if dep.Status.TerminatingReplicas != nil {
		terminatingPods = *dep.Status.TerminatingReplicas
	}

	if desiredReplicas == 0 && terminatingPods == 0 && dep.Status.Replicas == 0 {
		return DeploymentStatusStopped
	}

	return DeploymentStatusProgressing
}
