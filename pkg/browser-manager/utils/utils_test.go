package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var _ = Describe("Test Utility Functions", func() {

	It("AreResourcesEqual works as expected", func() {
		r1 := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
		}

		r2 := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
		}

		r3 := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
		}

		r4 := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
		}

		r5 := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
		}

		r6 := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("50m"),
			},
		}

		r7 := corev1.ResourceRequirements{}

		Expect(AreResourcesEqual(r1, r2)).To(BeTrue())
		Expect(AreResourcesEqual(r1, r3)).To(BeFalse())
		Expect(AreResourcesEqual(r4, r5)).To(BeTrue())
		Expect(AreResourcesEqual(r4, r6)).To(BeFalse())
		Expect(AreResourcesEqual(r7, r7)).To(BeTrue())
	})

	It("GetDeploymentStatus works as expected", func() {
		dep1Replicas := int32(1)
		dep1 := appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Replicas: &dep1Replicas,
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 1,
			},
		}

		dep2Replicas := int32(0)
		dep2 := appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Replicas: &dep2Replicas,
			},
			Status: appsv1.DeploymentStatus{
				Replicas: 0,
			},
		}

		dep3Replicas := int32(0)
		dep3Terminating := int32(1)
		dep3 := appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Replicas: &dep3Replicas,
			},
			Status: appsv1.DeploymentStatus{
				Replicas:            0,
				TerminatingReplicas: &dep3Terminating,
			},
		}

		dep4Replicas := int32(1)
		dep4 := appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Replicas: &dep4Replicas,
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 0,
			},
		}

		Expect(GetDeploymentStatus(&dep1)).To(Equal(DeploymentStatusReady))
		Expect(GetDeploymentStatus(&dep2)).To(Equal(DeploymentStatusStopped))
		Expect(GetDeploymentStatus(&dep3)).To(Equal(DeploymentStatusProgressing))
		Expect(GetDeploymentStatus(&dep4)).To(Equal(DeploymentStatusProgressing))

	})
})
