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
	"strings"

	utils "github.com/kubebrowser/operator/pkg/system-manager/utils"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	apiregv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/kubebrowser/operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const systemFinalizer = "core.kubebrowser.io/finalizer"

// Definitions to manage status conditions
const (
	// typeAvailableBrowser represents the status of the Deployment reconciliation
	typeAvailable = "Available"
	// typeDegradedBrowser represents the status used when the custom resource is deleted and the finalizer operations are yet to occur.
	typeDegraded = "Degraded"
)

// BrowserSystemReconciler reconciles a BrowserSystem object
type BrowserSystemReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=core.kubebrowser.io,resources=browsersystems,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.kubebrowser.io,resources=browsersystems/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.kubebrowser.io,resources=browsersystems/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=get;list;watch;create;update;patch;delete

func (r *BrowserSystemReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// get object, if not deployment means it was deleted
	system := &corev1alpha1.BrowserSystem{}
	err := r.Get(ctx, req.NamespacedName, system)
	if err != nil {
		return r.handleNotFound(&log, err)
	}

	// If no previous status, set initial status
	if len(system.Status.Conditions) == 0 {
		return r.handleNoConditions(ctx, system, &log)
	}

	// add finalizers for pre-delete hooks
	if !controllerutil.ContainsFinalizer(system, systemFinalizer) {
		return r.handleNoFinalizers(ctx, system, &log)
	}

	// Check if browser is marked to be deleted
	isMarkedToBeDeleted := system.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		return r.handleDeletion(ctx, req.NamespacedName, system, &log)
	}

	// Check if the browsercontroller deployment already exists, if not create a new one
	browserControllerDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: getBrowserControllerName(system.Name), Namespace: system.Namespace}, browserControllerDeployment)
	if err != nil {
		return r.handleNoBrowserController(ctx, system, &log, err)
	}

	/* browser-api Service	*/
	browserApiService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: getBrowserAPIName(system.Name), Namespace: system.Namespace}, browserApiService)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get Service for APIService")
		return ctrl.Result{}, err
	}

	// service not found && should be present => create new
	if err != nil && apierrors.IsNotFound(err) && shouldEnableAPIService(system.Spec.EnableApiService) {
		return r.createBrowserAPIService(ctx, system, &log)
	}

	// service found && shouldn't be present => delete
	if err == nil && !shouldEnableAPIService(system.Spec.EnableApiService) {
		return r.deleteBrowserAPIService(ctx, system, browserApiService, &log)
	}
	/* browser-api Service	*/

	/* browser-api Deployment	*/
	browserApiDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: getBrowserAPIName(system.Name), Namespace: system.Namespace}, browserApiDeployment)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get Deployment for APIService")
		return ctrl.Result{}, err
	}

	// deployment not found && should be present => create new
	if err != nil && apierrors.IsNotFound(err) && shouldEnableAPIService(system.Spec.EnableApiService) {
		return r.createBrowserAPIDeployment(ctx, system, &log)
	}

	// deployment found && shouldn't be present => delete
	if err == nil && !shouldEnableAPIService(system.Spec.EnableApiService) {
		return r.deleteBrowserAPIDeployment(ctx, system, browserApiDeployment, &log)
	}
	/* browser-api Deployment	*/

	/* browser-api APIService	*/
	browserApiAPIService := &apiregv1.APIService{}
	err = r.Get(ctx, types.NamespacedName{Name: getAPIServiceName(subresourceGV)}, browserApiAPIService)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get APIService")
		return ctrl.Result{}, err
	}

	// APIService not found && should be present => create new
	if err != nil && apierrors.IsNotFound(err) && shouldEnableAPIService(system.Spec.EnableApiService) {
		return r.createBrowserApiAPIService(ctx, system, &log)
	}

	// APIService found && shouldn't be present => delete
	if err == nil && !shouldEnableAPIService(system.Spec.EnableApiService) {
		return r.deleteBrowserApiAPIService(ctx, system, browserApiAPIService, &log)
	}
	/* browser-api APIService	*/

	// The following implementation will update the status
	meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Resources for BrowserSystem (%s) created successfully", system.Name)})

	system.Status.Status = utils.BrowserSystemStatusReady

	if err := r.Status().Update(ctx, system); err != nil {
		log.Error(err, "33 Failed to update BrowserSystem status 11")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BrowserSystemReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.BrowserSystem{}).
		Named("browsersystem").
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *BrowserSystemReconciler) handleNotFound(log *logr.Logger, err error) (ctrl.Result, error) {
	if apierrors.IsNotFound(err) {
		log.Info("browsersystem resource not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}
	// Error reading the object - requeue the request.
	log.Error(err, "1 Failed to get browser system")
	return ctrl.Result{}, err
}

func (r *BrowserSystemReconciler) handleNoConditions(ctx context.Context, system *corev1alpha1.BrowserSystem, log *logr.Logger) (ctrl.Result, error) {
	meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})

	system.Status.Status = utils.BrowserSystemStatusProgressing

	if err := r.Status().Update(ctx, system); err != nil {
		log.Error(err, "2 Failed to update Browser status 1")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *BrowserSystemReconciler) handleNoFinalizers(ctx context.Context, system *corev1alpha1.BrowserSystem, log *logr.Logger) (ctrl.Result, error) {
	log.Info("Adding Finalizer for Browser")
	if ok := controllerutil.AddFinalizer(system, systemFinalizer); !ok {
		err := fmt.Errorf("finalizer for Browser was not added")
		log.Error(err, "4 Failed to add finalizer for Browser")
		return ctrl.Result{}, err
	}

	if err := r.Update(ctx, system); err != nil {
		log.Error(err, "5 Failed to update custom resource to add finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *BrowserSystemReconciler) handleDeletion(ctx context.Context, key client.ObjectKey, system *corev1alpha1.BrowserSystem, log *logr.Logger) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(system, systemFinalizer) {
		log.Info("Performing Finalizer Operations for Browser before delete CR")

		// Let's add here a status "Downgrade" to reflect that this resource began its process to be terminated.
		meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeDegraded,
			Status: metav1.ConditionUnknown, Reason: "Finalizing",
			Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", system.Name)})

		if err := r.Status().Update(ctx, system); err != nil {
			log.Error(err, "6 Failed to update Browser status 2")
			return ctrl.Result{}, err
		}

		// execute finalizers
		err := r.doFinalizerOperations(ctx, system, log)
		if err != nil {
			log.Error(err, "Failed to perform deletion finalizer operation")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, key, system); err != nil {
			log.Error(err, "7 Failed to re-fetch browser")
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeDegraded,
			Status: metav1.ConditionTrue, Reason: "Finalizing",
			Message: fmt.Sprintf("Finalizer operations for browser %s were successfully accomplished", system.Name)})

		if err := r.Status().Update(ctx, system); err != nil {
			log.Error(err, "8 Failed to update Browser status 3")
			return ctrl.Result{}, err
		}

		log.Info("Removing Finalizer for Browser after successfully perform the operations")
		if ok := controllerutil.RemoveFinalizer(system, systemFinalizer); !ok {
			err := fmt.Errorf("finalizer for Browser was not removed")
			log.Error(err, "9 Failed to remove finalizer for Browser")
			return ctrl.Result{}, err
		}

		if err := r.Update(ctx, system); err != nil {
			log.Error(err, "10 Failed to remove finalizer for Browser")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// finalizeBrowser will perform the required operations before delete the CR.
func (r *BrowserSystemReconciler) doFinalizerOperations(ctx context.Context, system *corev1alpha1.BrowserSystem, log *logr.Logger) error {

	// The following implementation will raise an event
	r.Recorder.Event(system, "Warning", "Deleting",
		fmt.Sprintf("BrowserSystem %s is being deleted from the namespace %s",
			system.Name,
			system.Namespace))

	// delete api service if present
	err := r.deleteAnyAPIService(ctx, log)

	return err
}

func getLabelsForSystem(systemName string) map[string]string {
	var imageTag string
	image, err := getBrowserControllerImage()
	if err == nil {
		imageParts := strings.Split(image, ":")
		if len(imageParts) == 2 {
			imageTag = imageParts[1]
		}
	}
	return map[string]string{
		"app.kubernetes.io/instance":   systemName,
		"app.kubernetes.io/name":       "system",
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/managed-by": "BrowserSystemController",
	}
}

// takes system name and returns browser api name
func getBrowserAPIName(systemName string) string {
	return systemName + "-browser-api"
}

func shouldEnableAPIService(value *bool) bool {
	defaultValue := true
	if value == nil {
		return defaultValue
	}
	return *value
}
