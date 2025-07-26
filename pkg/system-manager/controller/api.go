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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/util/intstr"
	apiregv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"k8s.io/utils/ptr"
)

var subresourceGV = schema.GroupVersion{Group: "subresource.kubebrowser.io", Version: "v1alpha1"}

func (r *BrowserSystemReconciler) createBrowserAPIService(ctx context.Context, system *corev1alpha1.BrowserSystem, log *logr.Logger) (ctrl.Result, error) {

	// Define a new service
	service, err := r.getBrowserApiService(system)
	if err != nil {
		log.Error(err, "14 Failed to define new Service resource for System")

		// The following implementation will update the status
		meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
			Status: metav1.ConditionFalse, Reason: "Reconciling",
			Message: fmt.Sprintf("Failed to create Service for the browser system (%s): (%s)", system.Name, err)})

		if err := r.Status().Update(ctx, system); err != nil {
			log.Error(err, "15 Failed to update System status 5")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)

	if err = r.Create(ctx, service); err != nil {
		log.Error(err, "16 Failed to create new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil

}

func (r *BrowserSystemReconciler) deleteBrowserAPIService(ctx context.Context, system *corev1alpha1.BrowserSystem, service *corev1.Service, log *logr.Logger) (ctrl.Result, error) {
	log.Info("Deleting Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
	if err := r.Delete(ctx, service); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to delete service for api")

			// The following implementation will update the status
			meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to delete Service for the browser api (%s): (%s)", service.Name, err)})

			if err := r.Status().Update(ctx, system); err != nil {
				log.Error(err, "15 Failed to update System status 5")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
	}

	// deletion success move forward
	return ctrl.Result{}, nil
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

func (r *BrowserSystemReconciler) createBrowserAPIDeployment(ctx context.Context, system *corev1alpha1.BrowserSystem, log *logr.Logger) (ctrl.Result, error) {
	deployment, err := r.getBrowserApiDeployment(system)
	if err != nil {
		log.Error(err, "14 Failed to define new Deployment resource for System")

		// The following implementation will update the status
		meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
			Status: metav1.ConditionFalse, Reason: "Reconciling",
			Message: fmt.Sprintf("Failed to create Deployment for the browser system (%s): (%s)", system.Name, err)})

		if err := r.Status().Update(ctx, system); err != nil {
			log.Error(err, "15 Failed to update System status 5")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	log.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)

	if err = r.Create(ctx, deployment); err != nil {
		log.Error(err, "16 Failed to create new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *BrowserSystemReconciler) deleteBrowserAPIDeployment(ctx context.Context, system *corev1alpha1.BrowserSystem, deployment *appsv1.Deployment, log *logr.Logger) (ctrl.Result, error) {
	log.Info("Deleting Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
	if err := r.Delete(ctx, deployment); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to delete deployment for api")

			// The following implementation will update the status
			meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to delete Deployment for the browser api (%s): (%s)", deployment.Name, err)})

			if err := r.Status().Update(ctx, system); err != nil {
				log.Error(err, "15 Failed to update System status 5")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
	}

	// deletion success move forward
	return ctrl.Result{}, nil
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
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("200Mi"),
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

func (r *BrowserSystemReconciler) createBrowserApiAPIService(ctx context.Context, system *corev1alpha1.BrowserSystem, log *logr.Logger) (ctrl.Result, error) {
	apiService, err := r.getBrowserApiAPIService(system)
	if err != nil {
		log.Error(err, "14 Failed to define new APIService resource for System")

		// The following implementation will update the status
		meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
			Status: metav1.ConditionFalse, Reason: "Reconciling",
			Message: fmt.Sprintf("Failed to create APIService for the browser system (%s): (%s)", system.Name, err)})

		if err := r.Status().Update(ctx, system); err != nil {
			log.Error(err, "15 Failed to update System status 5")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	log.Info("Creating a new APIService", "APIService.Name", apiService.Name)

	if err = r.Create(ctx, apiService); err != nil {
		log.Error(err, "16 Failed to create new APIService", "APIService.Name", apiService.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *BrowserSystemReconciler) deleteBrowserApiAPIService(ctx context.Context, system *corev1alpha1.BrowserSystem, apiService *apiregv1.APIService, log *logr.Logger) (ctrl.Result, error) {
	log.Info("Deleting APIService", "Name", apiService.Name)
	if err := r.Delete(ctx, apiService); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to delete APIService for api")

			// The following implementation will update the status
			meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to delete APIService for the browser api (%s): (%s)", apiService.Name, err)})

			if err := r.Status().Update(ctx, system); err != nil {
				log.Error(err, "15 Failed to update System status 5")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
	}

	// deletion success move forward
	return ctrl.Result{}, nil
}

func (r *BrowserSystemReconciler) getBrowserApiAPIService(
	system *corev1alpha1.BrowserSystem) (*apiregv1.APIService, error) {

	serviceName := getBrowserAPIName(system.Name)
	ls := getLabelsForSystem(system.Name)

	apiService := &apiregv1.APIService{
		ObjectMeta: metav1.ObjectMeta{
			Name:   getAPIServiceName(subresourceGV),
			Labels: ls,
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
	return apiService, nil
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
