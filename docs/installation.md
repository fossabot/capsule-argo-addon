
# Installation

The Installation of the addon is only supported via Helm-Chart. Any other method is not officially supported.

## Requirements

The following is expected to be installed (including their CRDs)

- [Capsule](https://artifacthub.io/packages/helm/projectcapsule/capsule)
- [Argo(CD)](https://artifacthub.io/packages/helm/argo/argo-cd)

Without these the addon won't work.

### Capsule-Proxy

The [capsule-proxy](https://artifacthub.io/packages/helm/projectcapsule/capsule-proxy) is used to allow serviceaccounts to just see what they should see within the boundaries of your tenant. It is optional to use the proxy and it can be disabled via the [configuration](./config.md).

If you plan to use the capsule-proxy, we recommend installing a dedicated capsule-proxy instance for the addon, because Argo puts a lot of pressure on the proxy.

With the [Helm Chart](#helm) a dedicated capsule-proxy is already installed (exclusive CRDs) by default. Adjust this according to your needs and your setups.

We are working on a new feature for the capsule-proxy. This is required by this addon. Until this feature was implemented, you need to create an additional mapping:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: list-all-resources
rules:
  - apiGroups: ["*"]          # Allows access to all API groups
    resources: ["*"]          # Allows access to all resources within these API groups
    verbs: ["list"]           # Only allows the 'list' action
---
# ClusterRoleBinding that binds the ClusterRole to all ServiceAccounts
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: list-all-resources-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: list-all-resources     # The ClusterRole to bind
subjects:
  - kind: Group
    name: system:serviceaccounts   # This grants the role to all ServiceAccounts in the cluster
    apiGroup: rbac.authorization.k8s.io
```

### Capsule

You must add the namespace where the serviceaccounts are deployed to, to the capsuleUsers. For example:

```yaml
apiVersion: capsule.clastix.io/v1beta2
kind: CapsuleConfiguration
metadata:
  name: default
spec:
  ...
  # The default installation namespace
  userGroups:
  - system:serviceaccounts:capsule-argo-addon
  # Add other namespaces (etc..)
  - system:serviceaccounts:privileged-service-accounts
```

## Helm

[Artifact Hub](https://artifacthub.io/packages/helm/capsule-argo-addon/capsule-argo-addon)

Currently we support installation via Helm-Chart click the badge or [here](https://artifacthub.io/packages/helm/capsule-argo-addon/capsule-argo-addon) to view instructions and possible values on the chart.