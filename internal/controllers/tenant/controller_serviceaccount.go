package tenant

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	ccaerrrors "github.com/peak-scale/capsule-argo-addon/internal/errors"
	"github.com/peak-scale/capsule-argo-addon/internal/meta"
	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Creates Teanant Service Account with the given name and namespace
func (i *TenancyController) reconcileArgoServiceAccount(
	ctx context.Context,
	log logr.Logger,
	tenant *capsulev1beta2.Tenant,
) (token string, err error) {

	// Get Required default values
	serviceAccount := tenant.Name
	namespace := i.Settings.Get().Proxy.ServiceAccountNamespace

	// Verify if ServiceAccount-Namespace is declared on tenant-basis
	if ns := meta.TenantServiceAccountNamespace(tenant); ns != "" {
		namespace = ns
	}

	log.V(7).Info("reconciling serviceaccount", "serviceaccount", serviceAccount, "namespace", namespace)

	accountResource := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccount,
			Namespace: namespace,
		},
	}

	err = i.Client.Get(
		ctx,
		client.ObjectKey{
			Name:      accountResource.Name,
			Namespace: accountResource.Namespace,
		}, accountResource)
	if err != nil && !k8serrors.IsNotFound(err) {
		return "", err
	}

	// Decouple Object
	if !tenant.ObjectMeta.DeletionTimestamp.IsZero() {
		if meta.TenantDecoupleProject(tenant) && !k8serrors.IsNotFound(err) {
			_, err := controllerutil.CreateOrPatch(
				ctx,
				i.Client,
				accountResource,
				func() error {
					log.V(5).Info(
						"decoupling serviceaccount",
						"serviceaccount", accountResource.Name,
						"namespace", accountResource.Namespace)

					if err := i.DecoupleTenant(accountResource, tenant); err != nil {
						return err
					}

					return i.DecoupleTenant(accountResource, tenant)
				})
			if err != nil {
				return "", err
			}

			return "", nil
		}
	}

	if !meta.HasTenantOwnerReference(accountResource, tenant) {
		if !i.ForceTenant(tenant) && !k8serrors.IsNotFound(err) {
			log.V(5).Info(
				"proxy already present, not overriding",
				"serviceaccount", accountResource.Name,
				"namespace", accountResource.Namespace)

			return "", ccaerrrors.NewObjectAlreadyExistsError(accountResource)
		}
	}

	// Remove ServiceAccount if not enabled
	if !i.provisionProxyService(tenant) {
		log.V(7).Info("removing serviceaccount as owner", "serviceaccount", serviceAccount, "namespace", namespace)
		if err := i.removeServiceAccountOwner(ctx, log, tenant, namespace, serviceAccount); err != nil {
			return "", err
		}

		log.V(7).Info("lifecycling serviceaccount", "serviceaccount", serviceAccount, "namespace", namespace)
		err := i.Client.Delete(ctx, accountResource)
		if err != nil && !k8serrors.IsNotFound(err) {
			return "", fmt.Errorf("failed to lifecycle serviceaccount: %w", err)
		}
		return "", nil
	}

	log.V(7).Info("ensuring serviceaccount", "serviceaccount", serviceAccount, "namespace", namespace)

	// Create ServiceAccount
	_, err = controllerutil.CreateOrUpdate(ctx, i.Client, accountResource, func() (err error) {
		if accountResource.ObjectMeta.Labels == nil {
			accountResource.ObjectMeta.Labels = make(map[string]string)
		}
		accountResource.ObjectMeta.Labels = meta.TranslatorTrackingLabels(tenant)

		return meta.AddDynamicTenantOwnerReference(ctx, i.Client.Scheme(), accountResource, tenant)
	})
	if err != nil {
		return "", err
	}

	// Add ServiceAccount to Tenant-Spec
	err = i.addServiceAccountOwner(ctx, log, tenant, namespace, serviceAccount)
	if err != nil {
		return "", err
	}

	tokenResource := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccount,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}

	// Attempt to fetch the existing secret to ensure ResourceVersion is set if it exists
	err = i.Client.Get(ctx, client.ObjectKey{Name: serviceAccount, Namespace: namespace}, tokenResource)
	if err != nil && !k8serrors.IsNotFound(err) {
		// Return any error other than NotFound
		return "", err
	}

	log.V(7).Info(
		"ensuring serviceaccount token",
		"serviceaccount", serviceAccount,
		"secret", serviceAccount,
		"namespace", namespace)

	// Create Account Token
	_, err = controllerutil.CreateOrUpdate(ctx, i.Client, tokenResource, func() (err error) {
		tokenResource.ObjectMeta.Labels = meta.TranslatorTrackingLabels(tenant)

		if tokenResource.ObjectMeta.Annotations == nil {
			tokenResource.ObjectMeta.Annotations = make(map[string]string)
		}
		tokenResource.ObjectMeta.Annotations["kubernetes.io/service-account.name"] = serviceAccount

		if err := meta.AddDynamicTenantOwnerReference(ctx, i.Client.Scheme(), accountResource, tenant); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	log.V(7).Info(
		"extracting serviceaccount token",
		"serviceaccount", serviceAccount,
		"secret", serviceAccount,
		"namespace", namespace)

	var secret corev1.Secret
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = i.Client.Get(ctx, client.ObjectKey{
			Name:      tokenResource.Name,
			Namespace: namespace,
		}, &secret); err != nil {
			return err
		}

		t, exists := secret.Data["token"]
		if !exists {
			return err
		}

		token = string(t)
		return
	})
	if err != nil {
		return "", err
	}

	log.V(5).Info("serviceaccount reconciled", "serviceaccount", serviceAccount, "namespace", namespace)

	return token, nil
}

// Adds the given service account as an owner to the tenant
func (i *TenancyController) addServiceAccountOwner(
	ctx context.Context,
	log logr.Logger,
	tenant *capsulev1beta2.Tenant,
	namespace string,
	name string,
) (err error) {
	owner := capsulev1beta2.OwnerSpec{
		Kind: "ServiceAccount",
		Name: "system:serviceaccount:" + namespace + ":" + name,
	}

	// Check if the owner is already present
	for _, o := range tenant.Spec.Owners {
		if o.Kind == owner.Kind && o.Name == owner.Name {
			log.V(5).Info("serviceaccount already owner")
			return nil
		}
	}

	err = retry.RetryOnConflict(retry.DefaultBackoff, func() (conflict error) {
		_ = i.Client.Get(ctx, types.NamespacedName{Name: tenant.Name}, tenant)
		log.V(5).Info("adding serviceaccount as owner")

		tenant.Spec.Owners = append(tenant.Spec.Owners, owner)
		if conflict = i.Client.Update(ctx, tenant); err != nil {
			return err
		}
		return
	})

	return nil
}

// Removes a ServiceAccount from the ownerspec of a tenant
func (i *TenancyController) removeServiceAccountOwner(
	ctx context.Context,
	log logr.Logger,
	tenant *capsulev1beta2.Tenant,
	namespace string,
	name string,
) error {
	owner := capsulev1beta2.OwnerSpec{
		Kind: "ServiceAccount",
		Name: "system:serviceaccount:" + namespace + ":" + name,
	}

	// Check if the owner is already present
	present := false
	for _, o := range tenant.Spec.Owners {
		if o.Kind == owner.Kind && o.Name == owner.Name {
			present = true
			log.V(5).Info("serviceaccount still owner")
			break
		}
	}

	if !present {
		return nil
	}

	// Retry logic to avoid conflicts
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := i.Client.Get(ctx, types.NamespacedName{Name: tenant.Name}, tenant); err != nil {
			return err
		}

		// Filter out the ServiceAccount owner
		owners := capsulev1beta2.OwnerListSpec{}
		log.V(5).Info("removing serviceaccount as owner if it exists")
		for _, o := range tenant.Spec.Owners {
			if !(o.Kind == owner.Kind && o.Name == owner.Name) {
				owners = append(owners, o)
			}
		}
		tenant.Spec.Owners = owners

		// Update the tenant resource
		return i.Client.Update(ctx, tenant)
	})
}
