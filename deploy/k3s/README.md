# Single-engine K3s deployment

These resources run one engine and one Go management backend. Go watches the engine namespace and connects to the selected Pod IP. TLS validates `execution-engine.loomspan-engine.svc` as the server DNS identity and `spiffe://loomspan-cloud/management/backend` as the Go client URI SAN. The NetworkPolicy admits TCP port 8443 only from backend-labeled pods in `loomspan-management`. The Console additionally requires the environment's API key.

## Certificates and Console key

Install cert-manager and provision an environment-owned `loomspan-engine-transport` ClusterIssuer backed by that environment's dedicated transport CA. The issuer must include the trust certificate as `ca.crt` in the issued Secret. The two `Certificate` resources own the issuance and renewal of `engine-server-tls` and `management-client-tls`. The pods mount the complete Secrets read-only so projected credential updates can reach them. CA rollover requires a coordinated trust update.

Apply `deploy/k3s/namespaces.yaml` and populate both namespace-local `loomspan-console` Secrets with the same key, drawn from that environment's secret store. Use a different key for each environment. The Framework accepts 32 to 512 printable non-space ASCII characters. For an existing protected key file holding just the key, pass the file path without printing its contents:

```sh
kubectl --context "$KUBE_CONTEXT" apply -f deploy/k3s/namespaces.yaml
for namespace in loomspan-engine loomspan-management; do
  kubectl --context "$KUBE_CONTEXT" -n "$namespace" create secret generic loomspan-console \
    --from-file=api-key="$CONSOLE_KEY_FILE"
done
```

## Build and deploy

Use JDK 21 and Maven 3.9 or later. The Framework starter snapshot must include the per-namespace trace persistence field from Framework commit `2ca9caf4` or later. Build from the repository root:

```sh
mvn -f execution-engine/pom.xml verify
docker build -t loomspan-execution-engine:local execution-engine
docker build -t loomspan-management-backend:local management-backend
```

Import those tags into the disposable K3s node or provide registry-backed tags in your environment overlay. Apply the resources and wait for their certificates to become ready:

```sh
kubectl --context "$KUBE_CONTEXT" apply -k deploy/k3s
kubectl --context "$KUBE_CONTEXT" -n loomspan-engine wait certificate/engine-server --for=condition=Ready --timeout=120s
kubectl --context "$KUBE_CONTEXT" -n loomspan-management wait certificate/management-client --for=condition=Ready --timeout=120s
```

Go logs the watch-selected Pod UID and authenticated JVM ID, including whether a snapshot and the Console activity stream are connected. The first authenticated snapshot is available without any Cloud application or rollout probe. Routing additionally requires a ready, non-terminating pod and a matching namespace publication.

## Disposable observation check

Use a dedicated test cluster with no Cloud application installed. Build and import the opt-in observation Job image:

```sh
docker build --target observation-probe -t loomspan-observation-probe:local management-backend
kubectl --context "$KUBE_CONTEXT" apply -f deploy/k3s/verify-observation.yaml
kubectl --context "$KUBE_CONTEXT" -n loomspan-management logs -f job/loomspan-observation-probe
```

The Job checks the authorized first snapshot, the independent Console key, a fixture publish, and an explicit refresh with a namespace policy that differs from the default. It logs `READY_FOR_REPLACEMENT` after the original pod and JVM qualify for routing the fixture publication. Delete the dedicated engine pod named on that line to exercise replacement, then wait for the Job to complete. Compare the logged watched pod UIDs to Kubernetes.

The same probe image's `TestK3sEngineNetworkPeer` attempts a TCP connection using `LOOMSPAN_K3S_ENGINE_IP`. It expects success from a backend-labeled pod in the management namespace. With `LOOMSPAN_K3S_EXPECT_BLOCKED=true`, it expects failure. Run the blocked case from an unlabeled management pod and from a backend-labeled pod in another test namespace. Inside the engine pod, an HTTPS request to localhost without a client certificate must also fail.
