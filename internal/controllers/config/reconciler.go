/*
Copyright 2024.

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

package config

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	addonsv1alpha1 "github.com/peak-scale/capsule-argo-addon/api/v1alpha1"
	"github.com/peak-scale/capsule-argo-addon/internal/stores"
)

// CapsuleArgocdReconciler reconciles a CapsuleArgocd object
type ConfigReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Store    *stores.ConfigStore
	Recorder record.EventRecorder
	Log      logr.Logger
	Config   ReconcilerConfig
}

type ReconcilerConfig struct {
	SettingName string
}

//nolint:lll
//+kubebuilder:rbac:groups=config.projectcapsule.dev,resources=argocdaddons,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=config.projectcapsule.dev,resources=argocdaddons/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=config.projectcapsule.dev,resources=argocdaddons/finalizers,verbs=update

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&addonsv1alpha1.ArgoAddon{}).
		WithEventFilter(r.settingsNamePredicate()).
		Complete(r)
}

func (r *ConfigReconciler) settingsNamePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.GetName() == r.Config.SettingName
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectNew.GetName() == r.Config.SettingName
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetName() == r.Config.SettingName
		},
	}
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CapsuleArgocd object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *ConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("settings", req.Name)

	// Load the CapsuleArgocd object
	origin := &addonsv1alpha1.ArgoAddon{}
	if err := r.Get(ctx, req.NamespacedName, origin); err != nil {
		log.Error(err, "unable to fetch ArgoAddon")
		return ctrl.Result{}, client.IgnoreNotFound(err) // Ignore not found error
	}

	err := r.reconcile(ctx, log, r.Client, origin)
	if err != nil {
		log.Error(err, "failed to update settings")
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// First execttion of the controller to load the settings (without manager cache)
func (r *ConfigReconciler) Initialize(ctx context.Context, client client.Client) (err error) {
	origin := &addonsv1alpha1.ArgoAddon{}

	if err := client.Get(ctx, types.NamespacedName{Name: r.Config.SettingName}, origin); err != nil {
		return fmt.Errorf("could not load addon settings from '%s': %s", r.Config.SettingName, err)
	}

	err = r.reconcile(ctx, r.Log, client, origin)
	if err != nil {
		return fmt.Errorf("failed to update settings: %s", err)
	}

	return
}

// Reconcile Configuration
func (r *ConfigReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	client client.Client,
	origin *addonsv1alpha1.ArgoAddon,
) error {

	// Validate the Settings
	if err := r.validateSettings(ctx, client, &origin.Spec); err != nil {
		return fmt.Errorf("failed to validate settings: %v", err)
	}

	log.V(5).Info("Validated settings", "settings", origin.Spec)

	// Update the store with the new configuration
	r.Store.Update(&origin.Spec)

	// Update the status with the new configuration
	//origin.Status.Config = origin.Spec.DeepCopy().Config
	//if err := client.Status().Update(ctx, origin); err != nil {
	//	return fmt.Errorf("failed to update config status: %v", err)
	//}
	//
	//// Update the store with the new configuration
	//r.Store.Update(origin.Spec)

	return nil
}

// Validates configuration before it's applied as status and transferred to the store
// If validation fails, the configuration is not applied
func (r *ConfigReconciler) validateSettings(_ context.Context, _ client.Client, _ *addonsv1alpha1.ArgoAddonSpec) error {
	//r.Store.Update(origin)
	return nil
}
