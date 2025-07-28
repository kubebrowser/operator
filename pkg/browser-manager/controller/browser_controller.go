/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"os"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/kubebrowser/operator/api/v1alpha1"
	"github.com/kubebrowser/operator/pkg/browser-manager/utils"
)

const browserFinalizer = "core.kubebrowser.io/finalizer"

// Definitions to manage status conditions
const (
	// typeAvailableBrowser represents the status of the Deployment reconciliation
	typeAvailableBrowser = "Available"
	// typeDegradedBrowser represents the status used when the custom resource is deleted and the finalizer operations are yet to occur.
	typeDegradedBrowser = "Degraded"
)

// BrowserReconciler reconciles a Browser object
type BrowserReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// The following markers are used to generate the rules permissions (RBAC) on config/rbac using controller-gen
// when the command <make manifests> is executed.
// To know more about markers see: https://book.kubebuilder.io/reference/markers.html

// +kubebuilder:rbac:groups=core.kubebrowser.io,resources=browsers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.kubebrowser.io,resources=browsers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.kubebrowser.io,resources=browsers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// For further info:
// - About Operator Pattern: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
// - About Controllers: https://kubernetes.io/docs/concepts/architecture/controller/
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *BrowserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// get object, if not deployment means it was deleted
	browser := &corev1alpha1.Browser{}
	err := r.Get(ctx, req.NamespacedName, browser)
	if err != nil {
		return r.handleBrowserNotFound(&log, err)
	}

	// If no previous status, set initial status
	if len(browser.Status.Conditions) == 0 {
		return r.handleNoConditions(ctx, browser, &log)
	}

	// add finalizers for pre-delete hooks
	if !controllerutil.ContainsFinalizer(browser, browserFinalizer) {
		return r.handleNoFinalizers(ctx, browser, &log)
	}

	// Check if browser is marked to be deleted
	isBrowserMarkedToBeDeleted := browser.GetDeletionTimestamp() != nil
	if isBrowserMarkedToBeDeleted {
		return r.handleBrowserDeletion(ctx, req.NamespacedName, browser, &log)
	}

	// Check if the service already exists, if not create a new one
	service := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: browser.Name, Namespace: browser.Namespace}, service)
	if err != nil {
		return r.handleFailedToGetService(ctx, browser, &log, err)
	}

	// Check if the deployment already exists, if not create a new one
	deployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: browser.Name, Namespace: browser.Namespace}, deployment)
	if err != nil {
		return r.handleFailedToGetDeployment(ctx, browser, &log, err)
	}

	expectedReplicas := int32(-1)
	if browser.Spec.Started != nil && *browser.Spec.Started {
		expectedReplicas = int32(1)
	} else {
		expectedReplicas = int32(0)
	}

	if *deployment.Spec.Replicas != expectedReplicas {
		return r.handleDeploymentRepliasMismatch(ctx, browser, &log, deployment, &expectedReplicas, req.NamespacedName)
	}

	if !utils.AreResourcesEqual(browser.Spec.BrowserResources, deployment.Spec.Template.Spec.Containers[0].Resources) {
		return r.handleDeploymentResourcesMismatch(ctx, browser, &log, deployment, req.NamespacedName)
	}

	if err := r.Get(ctx, req.NamespacedName, browser); err != nil {
		log.Error(err, "32 Failed to re-fetch browser")
		return ctrl.Result{}, err
	}

	// The following implementation will update the status
	meta.SetStatusCondition(&browser.Status.Conditions, metav1.Condition{Type: typeAvailableBrowser,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Resources for Browser (%s) created successfully", browser.Name)})

	browser.Status.DeploymentStatus = utils.GetDeploymentStatus(deployment)

	if err := r.Status().Update(ctx, browser); err != nil {
		log.Error(err, "33 Failed to update Browser status 11")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// finalizeBrowser will perform the required operations before delete the CR.
func (r *BrowserReconciler) doFinalizerOperationsForBrowser(cr *corev1alpha1.Browser) {
	// Extra cleanup

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Browser %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
}

// deploymentForBrowser returns a Browser Deployment object
func (r *BrowserReconciler) deploymentForBrowser(
	browser *corev1alpha1.Browser) (*appsv1.Deployment, error) {
	ls := labelsForBrowser(browser.Name)

	// Get the Operand image
	image, err := imageForBrowser()
	if err != nil {
		return nil, err
	}

	var replicas int32
	if browser.Spec.Started != nil && *browser.Spec.Started {
		replicas = 1
	} else {
		replicas = 0
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      browser.Name,
			Namespace: browser.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Image:           image,
						Name:            "browser",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Resources:       browser.Spec.BrowserResources,
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
				},
			},
		},
	}

	// Set the ownerRef for the Deployment
	if err := ctrl.SetControllerReference(browser, dep, r.Scheme); err != nil {
		return nil, err
	}
	return dep, nil
}

// labelsForBrowser returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForBrowser(browserName string) map[string]string {
	var imageTag string
	image, err := imageForBrowser()
	if err == nil {
		imageParts := strings.Split(image, ":")
		if len(imageParts) == 2 {
			imageTag = imageParts[1]
		}
	}
	return map[string]string{
		"app.kubernetes.io/instance":   browserName,
		"app.kubernetes.io/name":       "browser",
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/managed-by": "BrowserController",
	}
}

// imageForBrowser gets the Operand image which is managed by this controller
// from the BROWSER_IMAGE environment variable defined in the config/manager/manager.yaml
func imageForBrowser() (string, error) {
	var browserEnvVar = "BROWSER_IMAGE"
	browserImage, exists := os.LookupEnv(browserEnvVar)
	if !exists {
		return "", fmt.Errorf("unable to find %s environment variable with the image", browserEnvVar)
	}

	return browserImage, nil
}

// serviceForBrowser returns a Browser Service object
func (r *BrowserReconciler) serviceForBrowser(
	browser *corev1alpha1.Browser) (*corev1.Service, error) {
	ls := labelsForBrowser(browser.Name)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      browser.Name,
			Namespace: browser.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: ls,
			Ports: []corev1.ServicePort{
				{Name: "browser", TargetPort: intstr.FromInt(utils.BrowserPortNumber), Port: utils.BrowserPortNumber},
				{Name: "vnc", TargetPort: intstr.FromInt(utils.VNCPortNumber), Port: utils.VNCPortNumber},
			},
		},
	}
	// Set the ownerRef for the Service
	if err := ctrl.SetControllerReference(browser, svc, r.Scheme); err != nil {
		return nil, err
	}
	return svc, nil
}

// SetupWithManager sets up the controller with the Manager.
// The whole idea is to be watching the resources that matter for the controller.
// When a resource that the controller is interested in changes, the Watch triggers
// the controller’s reconciliation loop, ensuring that the actual state of the resource
// matches the desired state as defined in the controller’s logic.
//
// Notice how we configured the Manager to monitor events such as the creation, update,
// or deletion of a Custom Resource (CR) of the Browser kind, as well as any changes
// to the Deployment that the controller manages and owns.
func (r *BrowserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Watch the Browser CR(s) and trigger reconciliation whenever it is created, updated, or deleted
		For(&corev1alpha1.Browser{}).
		Named("browser").
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *BrowserReconciler) handleBrowserNotFound(log *logr.Logger, err error) (ctrl.Result, error) {
	if apierrors.IsNotFound(err) {
		log.Info("browser resource not deployment. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}
	// Error reading the object - requeue the request.
	log.Error(err, "1 Failed to get browser")
	return ctrl.Result{}, err
}

func (r *BrowserReconciler) handleBrowserDeletion(ctx context.Context, key client.ObjectKey, browser *corev1alpha1.Browser, log *logr.Logger) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(browser, browserFinalizer) {
		log.Info("Performing Finalizer Operations for Browser before delete CR")

		// Let's add here a status "Downgrade" to reflect that this resource began its process to be terminated.
		meta.SetStatusCondition(&browser.Status.Conditions, metav1.Condition{Type: typeDegradedBrowser,
			Status: metav1.ConditionUnknown, Reason: "Finalizing",
			Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", browser.Name)})

		if err := r.Status().Update(ctx, browser); err != nil {
			log.Error(err, "6 Failed to update Browser status 2")
			return ctrl.Result{}, err
		}

		// execute finalizers
		r.doFinalizerOperationsForBrowser(browser)

		if err := r.Get(ctx, key, browser); err != nil {
			log.Error(err, "7 Failed to re-fetch browser")
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&browser.Status.Conditions, metav1.Condition{Type: typeDegradedBrowser,
			Status: metav1.ConditionTrue, Reason: "Finalizing",
			Message: fmt.Sprintf("Finalizer operations for browser %s were successfully accomplished", browser.Name)})

		if err := r.Status().Update(ctx, browser); err != nil {
			log.Error(err, "8 Failed to update Browser status 3")
			return ctrl.Result{}, err
		}

		log.Info("Removing Finalizer for Browser after successfully perform the operations")
		if ok := controllerutil.RemoveFinalizer(browser, browserFinalizer); !ok {
			err := fmt.Errorf("finalizer for Browser was not removed")
			log.Error(err, "9 Failed to remove finalizer for Browser")
			return ctrl.Result{}, err
		}

		if err := r.Update(ctx, browser); err != nil {
			log.Error(err, "10 Failed to remove finalizer for Browser")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *BrowserReconciler) handleFailedToGetService(ctx context.Context, browser *corev1alpha1.Browser, log *logr.Logger, err error) (ctrl.Result, error) {
	if apierrors.IsNotFound(err) {
		// Define a new service
		svc, err := r.serviceForBrowser(browser)
		if err != nil {
			log.Error(err, "11 Failed to define new Service resource for Browser")

			// The following implementation will update the status
			meta.SetStatusCondition(&browser.Status.Conditions, metav1.Condition{Type: typeAvailableBrowser,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Service for the browser (%s): (%s)", browser.Name, err)})

			if err := r.Status().Update(ctx, browser); err != nil {
				log.Error(err, "12 Failed to update Browser status 4")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		log.Info("Creating a new Service",
			"Serivice.Namespace", svc.Namespace, "Deployment.Name", svc.Name)
		if err = r.Create(ctx, svc); err != nil {
			log.Error(err, "12.5 Failed to create new Service",
				"Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
			return ctrl.Result{}, err
		}

		// Service created successfully, requeue the reconciliation so that and move forward for the next operations
		return ctrl.Result{Requeue: true}, nil
	}

	log.Error(err, "13 Failed to get Service")
	// Let's return the error for the reconciliation be re-triggered again
	return ctrl.Result{}, err
}

func (r *BrowserReconciler) handleFailedToGetDeployment(ctx context.Context, browser *corev1alpha1.Browser, log *logr.Logger, err error) (ctrl.Result, error) {
	if apierrors.IsNotFound(err) {
		// Define a new deployment
		dep, err := r.deploymentForBrowser(browser)
		if err != nil {
			log.Error(err, "14 Failed to define new Deployment resource for Browser")

			// The following implementation will update the status
			meta.SetStatusCondition(&browser.Status.Conditions, metav1.Condition{Type: typeAvailableBrowser,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Deployment for the browser (%s): (%s)", browser.Name, err)})

			if err := r.Status().Update(ctx, browser); err != nil {
				log.Error(err, "15 Failed to update Browser status 5")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		log.Info("Creating a new Deployment",
			"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		if err = r.Create(ctx, dep); err != nil {
			log.Error(err, "16 Failed to create new Deployment",
				"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		// set deployment status as progressing for new deployment
		browser.Status.DeploymentStatus = utils.DeploymentStatusProgressing
		if err := r.Status().Update(ctx, browser); err != nil {
			log.Error(err, "17 Failed to update Browser status 6")
			return ctrl.Result{}, err
		}

		// Deployment created successfully, requeue the reconciliation so that and move forward for the next operations
		return ctrl.Result{Requeue: true}, nil
	}

	log.Error(err, "18 Failed to get Deployment")
	// Let's return the error for the reconciliation be re-triggered again
	return ctrl.Result{}, err

}

func (r *BrowserReconciler) handleDeploymentRepliasMismatch(ctx context.Context, browser *corev1alpha1.Browser, log *logr.Logger, deployment *appsv1.Deployment, expectedReplicas *int32, key client.ObjectKey) (ctrl.Result, error) {
	deployment.Spec.Replicas = expectedReplicas

	if err := r.Update(ctx, deployment); err != nil {
		log.Error(err, "19 Failed to update Deployment",
			"Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)

		if err := r.Get(ctx, key, browser); err != nil {
			log.Error(err, "20 Failed to re-fetch browser")
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&browser.Status.Conditions, metav1.Condition{Type: typeAvailableBrowser,
			Status: metav1.ConditionFalse, Reason: "Starting",
			Message: fmt.Sprintf("Failed to start browser (%s): (%s)", browser.Name, err)})

		// updating deployment so update status to progressing
		browser.Status.DeploymentStatus = utils.DeploymentStatusProgressing

		if err := r.Status().Update(ctx, browser); err != nil {
			log.Error(err, "21 Failed to update Browser status 7")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *BrowserReconciler) handleDeploymentResourcesMismatch(ctx context.Context, browser *corev1alpha1.Browser, log *logr.Logger, deployment *appsv1.Deployment, key client.ObjectKey) (ctrl.Result, error) {
	deployment.Spec.Template.Spec.Containers[0].Resources = browser.Spec.BrowserResources

	if err := r.Update(ctx, deployment); err != nil {
		log.Error(err, "26 Failed to update Deployment",
			"Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)

		if err := r.Get(ctx, key, browser); err != nil {
			log.Error(err, "27 Failed to re-fetch browser")
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&browser.Status.Conditions, metav1.Condition{Type: typeAvailableBrowser,
			Status: metav1.ConditionFalse, Reason: "ResourcesChange",
			Message: fmt.Sprintf("Failed to update resources for browser (%s): (%s)", browser.Name, err)})

		// updating deployment so update status to progressing
		browser.Status.DeploymentStatus = utils.DeploymentStatusProgressing

		if err := r.Status().Update(ctx, browser); err != nil {
			log.Error(err, "28 Failed to update Browser status 9")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *BrowserReconciler) handleNoConditions(ctx context.Context, browser *corev1alpha1.Browser, log *logr.Logger) (ctrl.Result, error) {
	meta.SetStatusCondition(&browser.Status.Conditions, metav1.Condition{Type: typeAvailableBrowser, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})

	if err := r.Status().Update(ctx, browser); err != nil {
		log.Error(err, "2 Failed to update Browser status 1")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *BrowserReconciler) handleNoFinalizers(ctx context.Context, browser *corev1alpha1.Browser, log *logr.Logger) (ctrl.Result, error) {
	log.Info("Adding Finalizer for Browser")
	if ok := controllerutil.AddFinalizer(browser, browserFinalizer); !ok {
		err := fmt.Errorf("finalizer for Browser was not added")
		log.Error(err, "4 Failed to add finalizer for Browser")
		return ctrl.Result{}, err
	}

	if err := r.Update(ctx, browser); err != nil {
		log.Error(err, "5 Failed to update custom resource to add finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}
