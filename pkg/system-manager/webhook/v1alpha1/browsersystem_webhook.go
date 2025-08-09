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

package v1alpha1

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1alpha1 "github.com/kubebrowser/operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var browserSystemlog = logf.Log.WithName("browsersystem-resource")

// SetupBrowserSystemWebhookWithManager registers the webhook for Browser in the manager.
func SetupBrowserSystemWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&corev1alpha1.BrowserSystem{}).
		WithValidator(&BrowserSystemCustomValidator{client: mgr.GetClient()}).
		WithDefaulter(&BrowserSystemCustomDefaulter{
			DefaultEnableApiService:    true,
			DefaultEnableConsolePlugin: true,
		}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-core-kubebrowser-io-v1alpha1-browsersystem,mutating=true,failurePolicy=fail,sideEffects=None,groups=core.kubebrowser.io,resources=browsersystems,verbs=create;update,versions=v1alpha1,name=mbrowsersystem-v1alpha.kb.io,admissionReviewVersions=v1

// BrowserSystemCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind BrowserSystem when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type BrowserSystemCustomDefaulter struct {
	DefaultEnableApiService    bool
	DefaultEnableConsolePlugin bool
}

var _ webhook.CustomDefaulter = &BrowserSystemCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind BrowserSystem.
func (d *BrowserSystemCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	system, ok := obj.(*corev1alpha1.BrowserSystem)

	if !ok {
		return fmt.Errorf("expected an BrowserSystem object but got %T", obj)
	}
	browserSystemlog.Info("Defaulting for BrowserSystem", "name", system.GetName())

	// Set default values
	d.applyDefaults(system)
	return nil
}

// applyDefaults applies default values to BrowserSystem fields.
func (d *BrowserSystemCustomDefaulter) applyDefaults(system *corev1alpha1.BrowserSystem) {
	if system.Spec.EnableApiService == nil {
		system.Spec.EnableApiService = &d.DefaultEnableApiService
	}
	if system.Spec.EnableConsolePlugin == nil {
		system.Spec.EnableConsolePlugin = &d.DefaultEnableConsolePlugin
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-core-kubebrowser-io-v1alpha1-browsersystem,mutating=false,failurePolicy=fail,sideEffects=None,groups=core.kubebrowser.io,resources=browsersystems,verbs=create;update,versions=v1alpha1,name=vbrowsersystem-v1alpha1.kb.io,admissionReviewVersions=v1

// BrowserCustomValidator struct is responsible for validating the Browser resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type BrowserSystemCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
	client client.Client
}

var _ webhook.CustomValidator = &BrowserSystemCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Browser.
func (v *BrowserSystemCustomValidator) ValidateCreate(
	ctx context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	system, ok := obj.(*corev1alpha1.BrowserSystem)
	if !ok {
		return nil, fmt.Errorf("expected a BrowserSystem object but got %T", obj)
	}

	browserSystemlog.Info("Validation for BrowserSystem upon creation", "name", system.GetName())

	correctNamespace, err := getNamespace()
	if err != nil {
		return nil, fmt.Errorf("internal error verifying browsersystem namespace: %s", err)
	}

	if system.GetNamespace() != correctNamespace {
		return nil, fmt.Errorf("invalid namespace for browsersystem: expected namespace='%s'", correctNamespace)
	}

	browserSystemList := &corev1alpha1.BrowserSystemList{}
	err = v.client.List(ctx, browserSystemList, &client.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("internal error when listing browsersystems: %s", err)
	}

	if len(browserSystemList.Items) != 0 {
		return nil, fmt.Errorf("only one browsersystem allowed per cluster")
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Browser.
func (v *BrowserSystemCustomValidator) ValidateUpdate(_ context.Context,
	oldObj,
	newObj runtime.Object,
) (admission.Warnings, error) {
	system, ok := newObj.(*corev1alpha1.BrowserSystem)
	if !ok {
		return nil, fmt.Errorf("expected a BrowserSystem object for the newObj but got %T", newObj)
	}
	browserSystemlog.Info("Validation for BrowserSystem upon update", "name", system.GetName())

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Browser.
func (v *BrowserSystemCustomValidator) ValidateDelete(
	_ context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	system, ok := obj.(*corev1alpha1.BrowserSystem)
	if !ok {
		return nil, fmt.Errorf("expected a BrowserSystem object but got %T", obj)
	}
	browserSystemlog.Info("Validation for BrowserSystem upon deletion", "name", system.GetName())

	return nil, nil
}

func getNamespace() (string, error) {
	envName := "NAMESPACE"
	ns, exists := os.LookupEnv(envName)
	if !exists {
		return "", fmt.Errorf("missing '%s' environment variable", envName)
	}

	return ns, nil
}
