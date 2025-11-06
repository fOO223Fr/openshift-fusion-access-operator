# Building and Deploying Fusion Access Operator

This guide covers the complete procedure to build and deploy the Fusion Access Operator on AWS/OpenShift clusters.

**📖 For information about build issues and fixes, see [BUILD_ISSUES_AND_FIXES.md](BUILD_ISSUES_AND_FIXES.md)**

## Prerequisites

### Required Tools

- **Go 1.21+**
- **podman** (or docker) - Container build tool
- **oc** CLI - OpenShift command-line tool
- **jq** - JSON parser (for credential extraction)
- **Git** - Version control

### Required Credentials

#### 1. IBM Entitlement Key
- **Purpose**: Access IBM Storage Scale images from `cp.icr.io`
- **Obtain**: https://access.ibmfusion.eu/
- **Usage**: Create secret `fusion-pullsecret` in `ibm-fusion-access` namespace (see Post-Deployment section)

#### 2. Quay.io Credentials
- **Purpose**: Push operator images to your quay.io namespace
- **Obtain**: Create account at https://quay.io and generate API token
- **Setup**: `podman login quay.io`

#### 3. OpenShift Cluster Access
- **Setup**: `oc login --server=https://your-cluster:6443`
- **Permissions**: Cluster-admin or sufficient RBAC for CRDs, CatalogSource, Subscription

## Quick Start (Automated)

The easiest way to build and deploy:

```bash
# 1. Set environment variables
export REGISTRY="quay.io/your-username"
export VERSION="6.7.6"

# 2. Login to quay.io
podman login quay.io

# 3. Verify cluster access
oc cluster-info

# 4. Run automated build and deployment
./scripts/fusion-access-operator-build.sh
```

The script automatically:
- Builds all images (operator, console, devicefinder, bundle, catalog)
- Pushes images to your registry
- Sets up pull secrets
- Creates CatalogSource
- Installs operator via Subscription

## Manual Build Process

### Step 1: Set Environment Variables

```bash
export REGISTRY="quay.io/your-username"
export VERSION="6.7.6"
export IMAGE_TAG_BASE="${REGISTRY}/openshift-fusion-access"
```

### Step 2: Build Images

```bash
# Generate manifests and bundle
make manifests bundle generate

# Build all images
make docker-build
make console-build
make devicefinder-docker-build
make bundle-build
make catalog-build
```

### Step 3: Push Images

```bash
make docker-push
make console-push
make devicefinder-docker-push
make bundle-push
make catalog-push
```

### Step 4: Setup Pull Secrets

If using quay.io (private repositories), create pull secret for CatalogSource:

```bash
oc create secret docker-registry quay-pull-secret \
  --docker-server=quay.io \
  --docker-username=<your-username> \
  --docker-password=<your-token> \
  --docker-email="" \
  -n openshift-marketplace

oc patch serviceaccount default -n openshift-marketplace \
  --type='json' \
  -p='[{"op": "add", "path": "/imagePullSecrets/-", "value": {"name": "quay-pull-secret"}}]'
```

### Step 5: Install CatalogSource

```bash
make catalog-install
```

### Step 6: Create Subscription

```bash
# Create namespace
oc create namespace ibm-fusion-access

# Create OperatorGroup
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: fusion-access-operator-group
  namespace: ibm-fusion-access
spec:
  upgradeStrategy: Default
EOF

# Create Subscription
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: openshift-fusion-access-operator
  namespace: ibm-fusion-access
spec:
  channel: fast
  installPlanApproval: Automatic
  name: openshift-fusion-access-operator
  source: test-openshift-fusion-access-operator
  sourceNamespace: openshift-marketplace
EOF
```

## Post-Deployment Configuration

### Create IBM Entitlement Secret

**Critical**: The operator requires IBM entitlement credentials to pull IBM Storage Scale images:

```bash
oc create secret generic fusion-pullsecret \
  --from-literal=ibm-entitlement-key='<YOUR_IBM_ENTITLEMENT_KEY>' \
  -n ibm-fusion-access
```

**Where to get IBM Entitlement Key**:
- Visit: https://access.ibmfusion.eu/
- Log in with your IBM account
- Navigate to entitlement keys section
- Copy your entitlement key

### Create FusionAccess Resource

```bash
cat <<EOF | oc apply -f -
apiVersion: fusion.storage.openshift.io/v1alpha1
kind: FusionAccess
metadata:
  name: fusionaccess-object
  namespace: ibm-fusion-access
spec:
  storageScaleVersion: "v5.2.3.1"
  storageDeviceDiscovery:
    create: true
EOF
```

## Verification

```bash
# Check operator pod status
oc get pods -n ibm-fusion-access

# Check CSV status
oc get csv -n ibm-fusion-access

# Check operator status
oc get operators.operators.coreos.com -n ibm-fusion-access

# Check FusionAccess resource
oc get fusionaccess -n ibm-fusion-access
```

## Image Registries

| Registry | Purpose | Authentication |
|----------|---------|---------------|
| **quay.io** | Your operator images | Your quay.io credentials |
| **cp.icr.io** | IBM Storage Scale images | IBM entitlement key |
| **registry.redhat.io** | Red Hat base images | Red Hat account (usually public) |

## Troubleshooting

### CatalogSource ImagePullBackOff

```bash
# Verify pull secret exists
oc get secret quay-pull-secret -n openshift-marketplace

# Verify image is accessible
podman pull quay.io/your-username/openshift-fusion-access-catalog:6.7.6
```

### CatalogSource TRANSIENT_FAILURE

```bash
# Check pod status and logs
oc get pods -n openshift-marketplace -l olm.catalogSource=test-openshift-fusion-access-operator
oc logs -n openshift-marketplace <pod-name>

# Recreate CatalogSource
oc delete catalogsource test-openshift-fusion-access-operator -n openshift-marketplace
make catalog-install
```

### Subscription Resolution Failed

- Ensure CatalogSource pod is running and READY
- Check CatalogSource connection state: `oc get catalogsource test-openshift-fusion-access-operator -n openshift-marketplace -o yaml`
- Verify PackageManifest exists: `oc get packagemanifests -n openshift-marketplace openshift-fusion-access-operator`

### IBM Image Pull Errors

```bash
# Verify IBM entitlement secret exists
oc get secret fusion-pullsecret -n ibm-fusion-access

# Verify secret contains ibm-entitlement-key
oc get secret fusion-pullsecret -n ibm-fusion-access -o jsonpath='{.data.ibm-entitlement-key}' | base64 -d
```

## Cleanup

To remove all resources created by the build script:

```bash
make clean-docker
```

This removes:
- Subscription, CSV, Operator
- OperatorGroup
- CatalogSource
- Namespace `ibm-fusion-access`
- All associated resources (handles finalizers)

## Environment Variables Reference

### Required
- `REGISTRY` - Your container registry (e.g., `quay.io/your-username`)
- `VERSION` - Operator version (e.g., `6.7.6`)

### Optional
- `IMAGE_TAG_BASE` - Base for image tags (defaults to `${REGISTRY}/openshift-fusion-access`)
- `OPERATOR_IMG` - Operator image URL
- `CONSOLE_PLUGIN_IMAGE` - Console plugin image URL
- `DEVICEFINDER_IMAGE` - Devicefinder image URL
- `BUNDLE_IMG` - Bundle image URL
- `CATALOG_IMG` - Catalog image URL
- `CHANNELS` - Bundle channels (default: `fast`)
- `CONTAINER_TOOL` - Container tool to use (default: `podman`)

## Additional Resources

- Main README: See [README.md](README.md) for operator overview and development guide
- IBM Fusion Access: https://access.ibmfusion.eu/
- Red Hat Documentation: https://docs.redhat.com/en/documentation/openshift_container_platform/4.19/html/virtualization/virtualization-with-ibm-fusion-access-for-san

