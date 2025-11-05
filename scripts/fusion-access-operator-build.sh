#!/bin/bash
set -x -e -o pipefail

CATALOGSOURCE="test-openshift-fusion-access-operator"
NS="ibm-fusion-access"
OPERATOR="openshift-fusion-access-operator"
VERSION="${VERSION:-6.6.6}"
REGISTRY="${REGISTRY:-kuemper.int.rhx/bandini}"

wait_for_resource() {
    local resource_type=$1  # Either "packagemanifest", "operator", or "csv"
    local name=$2           # Name of the resource (e.g., Operator or CSV)
    local namespace=$3      # Namespace (optional, required for CSV and Operator)
    local label=$4          # Label selector (only for packagemanifests)
    local max_retries=${5:-60}  # Maximum retries (default: 60 = 10 minutes)
    local retry_count=0

    echo "⏳ Waiting for $resource_type: $name"
    while [ $retry_count -lt $max_retries ]; do
        set +e
        if [[ "$resource_type" == "packagemanifest" ]]; then
            oc get -n openshift-marketplace packagemanifests -l "catalog=${label}" --field-selector "metadata.name=${name}" &> /dev/null
        elif [[ "$resource_type" == "operator" ]]; then
            oc get operators.operators.coreos.com "${name}.${namespace}" &> /dev/null
        elif [[ "$resource_type" == "csv" ]]; then
            STATUS=$(oc get csv "$name" -n "$namespace" -o jsonpath='{.status.phase}' 2>/dev/null)
            if [[ "$STATUS" == "Succeeded" ]]; then
                echo "✅ Operator installation completed successfully!"
                break
            fi
            echo "⏳ Operator installation in progress... (Current status: ${STATUS:-Not Found}, attempt $((retry_count + 1))/$max_retries)"
        else
            echo "❌ Unknown resource type: $resource_type"
            return 1
        fi
        ret=$?
        set -e

        if [[ $ret -eq 0 && "$resource_type" != "csv" ]]; then
            echo "✅ $resource_type: $name is available!"
            break
        fi

        retry_count=$((retry_count + 1))
        sleep 10
    done

    if [ $retry_count -eq $max_retries ]; then
        echo "❌ Error: $resource_type $name was not available after $max_retries attempts (10 minutes)"
        return 1
    fi
}

wait_for_catalogsource_ready() {
    local catalogsource_name=$1
    local namespace=${2:-openshift-marketplace}
    local max_retries=${3:-60}  # Maximum retries (default: 60 = 10 minutes)
    local retry_count=0

    echo "⏳ Waiting for CatalogSource ${catalogsource_name} pod to be ready..."
    while [ $retry_count -lt $max_retries ]; do
        set +e
        # Check if CatalogSource exists
        oc get catalogsource "${catalogsource_name}" -n "${namespace}" &> /dev/null
        cs_exists=$?
        
        # Check if pod exists and is ready
        POD_STATUS=$(oc get pod -n "${namespace}" -l "olm.catalogSource=${catalogsource_name}" -o jsonpath='{.items[0].status.phase}' 2>/dev/null)
        POD_READY=$(oc get pod -n "${namespace}" -l "olm.catalogSource=${catalogsource_name}" -o jsonpath='{.items[0].status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)
        POD_NAME=$(oc get pod -n "${namespace}" -l "olm.catalogSource=${catalogsource_name}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        set -e

        if [ $cs_exists -ne 0 ]; then
            echo "⚠️  CatalogSource ${catalogsource_name} not found in namespace ${namespace}, retrying... (attempt $((retry_count + 1))/$max_retries)"
        elif [ -z "${POD_NAME}" ]; then
            echo "⏳ CatalogSource pod not found yet, waiting... (attempt $((retry_count + 1))/$max_retries)"
        elif [ "${POD_STATUS}" != "Running" ]; then
            echo "⏳ CatalogSource pod ${POD_NAME} status: ${POD_STATUS}, waiting... (attempt $((retry_count + 1))/$max_retries)"
        elif [ "${POD_READY}" != "True" ]; then
            echo "⏳ CatalogSource pod ${POD_NAME} not ready yet, waiting... (attempt $((retry_count + 1))/$max_retries)"
        else
            echo "✅ CatalogSource ${catalogsource_name} pod ${POD_NAME} is ready!"
            # Additional check: verify CatalogSource status shows it's healthy
            set +e
            CS_STATUS=$(oc get catalogsource "${catalogsource_name}" -n "${namespace}" -o jsonpath='{.status.connectionState.lastObservedState}' 2>/dev/null)
            if [ "${CS_STATUS}" = "READY" ]; then
                echo "✅ CatalogSource ${catalogsource_name} is READY!"
                break
            else
                echo "⏳ CatalogSource connection state: ${CS_STATUS:-Unknown}, waiting... (attempt $((retry_count + 1))/$max_retries)"
            fi
            set -e
            # If pod is ready, give it a moment and proceed
            sleep 5
            break
        fi

        retry_count=$((retry_count + 1))
        sleep 10
    done

    if [ $retry_count -eq $max_retries ]; then
        echo "❌ Error: CatalogSource ${catalogsource_name} pod was not ready after $max_retries attempts (10 minutes)"
        echo "Checking CatalogSource status:"
        oc get catalogsource "${catalogsource_name}" -n "${namespace}" -o yaml || true
        echo "Checking CatalogSource pod:"
        oc get pod -n "${namespace}" -l "olm.catalogSource=${catalogsource_name}" || true
        return 1
    fi
}

apply_subscription() {
    echo "Creating/updating namespace and subscription resources..."
    # Delete existing subscription if it exists (this is safe to do)
    oc delete -n ${NS} subscription/${OPERATOR} || /bin/true
    # Note: We do NOT delete the CatalogSource here - it must exist in openshift-marketplace
    # for the subscription to work. The CatalogSource is created by 'catalog-install' above.
    
    oc apply -f - <<EOF
    apiVersion: v1
    kind: Namespace
    metadata:
      name: ${NS}
    spec:
EOF
    oc apply -f - <<EOF
    apiVersion: operators.coreos.com/v1
    kind: OperatorGroup
    metadata:
      name: fusion-access-operator-group
      namespace: ${NS}
    spec:
      upgradeStrategy: Default
EOF
    oc apply -f - <<EOF
    apiVersion: operators.coreos.com/v1alpha1
    kind: Subscription
    metadata:
      name: ${OPERATOR}
      namespace: ${NS}
    spec:
      channel: fast
      installPlanApproval: Automatic
      name: ${OPERATOR}
      source: ${CATALOGSOURCE}
      sourceNamespace: openshift-marketplace
EOF
}

if [[ -n $(git status --porcelain) ]]; then
    echo "Uncommitted changes detected."
    exit 1
fi

echo "Checking for cluster reachability:"
OUT=$(oc cluster-info 2>&1)
ret=$?
if [ $ret -ne 0 ]; then
    echo "Could not reach cluster: ${OUT}"
    exit 1
fi

make VERSION=${VERSION} IMAGE_TAG_BASE=${REGISTRY}/openshift-fusion-access CHANNELS=fast USE_IMAGE_DIGESTS="" \
    manifests bundle generate docker-build docker-push bundle-build bundle-push console-build console-push \
    devicefinder-docker-build devicefinder-docker-push catalog-build catalog-push catalog-install

echo "Waiting for CatalogSource to be ready before proceeding..."
wait_for_catalogsource_ready "${CATALOGSOURCE}" "openshift-marketplace"

wait_for_resource "packagemanifest" "${OPERATOR}" "" "${CATALOGSOURCE}"
apply_subscription
wait_for_resource "operator" "${OPERATOR}" "${NS}"

echo "⏳ Waiting for Subscription to install CSV..."
MAX_RETRIES=60
RETRY_COUNT=0
while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    set +e
    INSTALLED_CSV=$(oc get subscription "${OPERATOR}" -n "${NS}" -o jsonpath='{.status.installedCSV}' 2>/dev/null)
    ret=$?
    set -e
    
    if [ $ret -ne 0 ]; then
        echo "⚠️  Subscription ${OPERATOR} not found in namespace ${NS}, retrying... (attempt $((RETRY_COUNT + 1))/$MAX_RETRIES)"
    elif [ -n "${INSTALLED_CSV}" ]; then
        echo "✅ CSV installed: ${INSTALLED_CSV}"
        break
    else
        echo "⏳ Waiting for CSV to be installed... (attempt $((RETRY_COUNT + 1))/$MAX_RETRIES)"
        # Check subscription status for debugging
        set +e
        SUB_STATUS=$(oc get subscription "${OPERATOR}" -n "${NS}" -o jsonpath='{.status.conditions[?(@.type=="CatalogSourcesUnhealthy")].message}' 2>/dev/null || echo "")
        RESOLUTION_ERROR=$(oc get subscription "${OPERATOR}" -n "${NS}" -o jsonpath='{.status.conditions[?(@.type=="ResolutionFailed")].message}' 2>/dev/null || echo "")
        if [ -n "${SUB_STATUS}" ]; then
            echo "   Subscription status: ${SUB_STATUS}"
        fi
        if [ -n "${RESOLUTION_ERROR}" ]; then
            echo "   ⚠️  Resolution error: ${RESOLUTION_ERROR}"
            # If there's a resolution error, check CatalogSource pod status
            CS_POD=$(oc get pod -n openshift-marketplace -l "olm.catalogSource=${CATALOGSOURCE}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
            if [ -n "${CS_POD}" ]; then
                CS_POD_STATUS=$(oc get pod -n openshift-marketplace "${CS_POD}" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
                echo "   CatalogSource pod ${CS_POD} status: ${CS_POD_STATUS}"
            fi
        fi
        set -e
    fi
    
    RETRY_COUNT=$((RETRY_COUNT + 1))
    sleep 10
done

if [ -z "${INSTALLED_CSV}" ]; then
    echo "❌ Error: CSV was not installed after $MAX_RETRIES attempts (10 minutes)"
    echo "Checking subscription status:"
    oc get subscription "${OPERATOR}" -n "${NS}" -o yaml || true
    exit 1
fi

wait_for_resource "csv" "${INSTALLED_CSV}" "${NS}"
