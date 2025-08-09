package controller

import (
	"context"
	"fmt"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/kubebrowser/operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/util/intstr"
	apiregv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"k8s.io/utils/ptr"
)

var subresourceGV = schema.GroupVersion{Group: "subresource.kubebrowser.io", Version: "v1alpha1"}

func (r *BrowserSystemReconciler) reconcileBrowserApi(
	ctx context.Context,
	system *corev1alpha1.BrowserSystem,
	log *logr.Logger,
) (ReconcileResult, error) {
	/* browser-api Service	*/
	browserApiService := &corev1.Service{}
	err := r.Get(ctx,
		types.NamespacedName{Name: getBrowserAPIName(system.Name), Namespace: system.Namespace},
		browserApiService,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if shouldEnableAPIService(system.Spec.EnableApiService) {
				// service not found && should be present => create new
				err := r.createService(ctx, system, log, ForApi)
				if err != nil {
					return ReconciledError, err
				}
				return ReconciledUpdated, nil
			}
		} else {
			log.Error(err, "failed to get service", "for", ForApi)
			return ReconciledError, err
		}
	} else if !shouldEnableAPIService(system.Spec.EnableApiService) {
		// service found but shouldn't be present
		err := r.deleteResource(ctx, system, browserApiService, log)
		if err != nil {
			return ReconciledError, err
		}
		return ReconciledUpdated, nil
	}
	/* browser-api Service	*/

	/* browser-api Deployment	*/
	browserApiDeployment := &appsv1.Deployment{}
	err = r.Get(ctx,
		types.NamespacedName{Name: getBrowserAPIName(system.Name), Namespace: system.Namespace},
		browserApiDeployment,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if shouldEnableAPIService(system.Spec.EnableApiService) {
				// deployment not found && should be present => create new
				err := r.createDeployment(ctx, system, log, ForApi)
				if err != nil {
					return ReconciledError, err
				}
				return ReconciledUpdated, nil
			}
		} else {
			log.Error(err, "Failed to get deployment", "for", ForApi)
			return ReconciledError, err
		}
	} else if !shouldEnableAPIService(system.Spec.EnableApiService) {
		// deployment found but shouldn't be present
		err := r.deleteResource(ctx, system, browserApiDeployment, log)
		if err != nil {
			return ReconciledError, err
		}
		return ReconciledUpdated, nil
	}
	/* browser-api Deployment	*/

	/* browser-api APIService	*/
	browserApiAPIService := &apiregv1.APIService{}
	err = r.Get(ctx, types.NamespacedName{Name: getAPIServiceName(subresourceGV)}, browserApiAPIService)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if shouldEnableAPIService(system.Spec.EnableApiService) {
				// apiservice not found && should be present => create new
				err := r.createBrowserApiAPIService(ctx, system, log)
				if err != nil {
					return ReconciledError, err
				}
				return ReconciledUpdated, err
			}
		} else {
			log.Error(err, "Failed to get apiservice")
			return ReconciledError, err
		}
	} else if !shouldEnableAPIService(system.Spec.EnableApiService) {
		// apiservice found but shouldn't be present
		err := r.deleteResource(ctx, system, browserApiAPIService, log)
		if err != nil {
			return ReconciledError, err
		}
		return ReconciledUpdated, nil
	}
	/* browser-api APIService	*/

	return ReconciledOk, nil
}

func (r *BrowserSystemReconciler) getBrowserApiService(
	system *corev1alpha1.BrowserSystem) (*corev1.Service, error) {
	ls := getLabelsForSystem(system.Name)

	name := getBrowserAPIName(system.Name)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   system.Namespace,
			Labels:      ls,
			Annotations: map[string]string{"service.beta.openshift.io/serving-cert-secret-name": getBrowserAPICertsName(name)},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": name,
			},
			Ports: []corev1.ServicePort{{Port: 443, TargetPort: intstr.FromInt(8443), Protocol: "TCP"}},
		},
	}
	if err := ctrl.SetControllerReference(system, service, r.Scheme); err != nil {
		return nil, err
	}
	return service, nil
}

func getBrowserAPICertsName(browserApiServiceName string) string {
	return browserApiServiceName + "-certs"
}

func (r *BrowserSystemReconciler) getBrowserApiDeployment(
	system *corev1alpha1.BrowserSystem) (*appsv1.Deployment, error) {

	ls := getLabelsForSystem(system.Name)
	name := getBrowserAPIName(system.Name)
	image, err := getBrowserAPIImage()
	if err != nil {
		return nil, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: system.Namespace,
			Labels:    ls,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:           image,
						Name:            "browser-api",
						ImagePullPolicy: corev1.PullIfNotPresent,
						VolumeMounts:    []corev1.VolumeMount{{Name: "certs", MountPath: "/tls", ReadOnly: true}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("250Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("2"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             ptr.To(true),
							AllowPrivilegeEscalation: ptr.To(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
						},
					},
					},
					Volumes: []corev1.Volume{{Name: "certs", VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: getBrowserAPICertsName(name),
						}}}},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(system, deployment, r.Scheme); err != nil {
		return nil, err
	}
	return deployment, nil
}

func getBrowserAPIImage() (string, error) {
	var envVar = "BROWSER_API_IMAGE"
	image, exists := os.LookupEnv(envVar)
	if !exists {
		return "", fmt.Errorf("unable to find %s environment variable with the image", envVar)
	}

	return image, nil
}

func (r *BrowserSystemReconciler) createBrowserApiAPIService(ctx context.Context, system *corev1alpha1.BrowserSystem, log *logr.Logger) error {
	apiService := r.getBrowserApiAPIService(system)
	log.Info("Creating a new APIService", "APIService.Name", apiService.Name)
	if err := r.Create(ctx, apiService); err != nil {
		log.Error(err, "16 Failed to create new APIService", "APIService.Name", apiService.Name)
		return err
	}

	return nil
}

func (r *BrowserSystemReconciler) getBrowserApiAPIService(
	system *corev1alpha1.BrowserSystem) *apiregv1.APIService {

	serviceName := getBrowserAPIName(system.Name)
	ls := getLabelsForSystem(system.Name)

	apiService := &apiregv1.APIService{
		ObjectMeta: metav1.ObjectMeta{
			Name:   getAPIServiceName(subresourceGV),
			Labels: ls,
			Annotations: map[string]string{
				"service.beta.openshift.io/inject-cabundle": "true",
			},
		},
		Spec: apiregv1.APIServiceSpec{
			Version:              subresourceGV.Version,
			Group:                subresourceGV.Group,
			GroupPriorityMinimum: 2000,
			VersionPriority:      10,
			Service: &apiregv1.ServiceReference{
				Name:      serviceName,
				Namespace: system.Namespace,
			},
		},
	}

	// namespace scoped resource cannot set owner reference to a cluster scoped resource -_- => cleanup in finalizer

	// if err := ctrl.SetControllerReference(system, apiService, r.Scheme); err != nil {
	// 	return nil, err
	// }
	return apiService
}

func (r *BrowserSystemReconciler) deleteAnyAPIService(ctx context.Context, log *logr.Logger) error {
	apiService := &apiregv1.APIService{}
	err := r.Get(ctx, types.NamespacedName{Name: getAPIServiceName(subresourceGV)}, apiService)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		} else {
			return err
		}
	}

	log.Info("Deleting APIService for api")

	err = r.Delete(ctx, apiService)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		} else {
			return err
		}
	}

	return nil
}

func getAPIServiceName(gv schema.GroupVersion) string {
	return fmt.Sprintf("%s.%s", gv.Version, gv.Group)
}

// takes system name and returns browser api name
func getBrowserAPIName(systemName string) string {
	return systemName + "-browser-api"
}
