package controller

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/kubebrowser/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

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
	if err := r.Client.Delete(ctx, service); err != nil {
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
