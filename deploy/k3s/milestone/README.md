# Restored skill milestone

The probe uses the real Go publication and invocation services, Kubernetes watch,
Console, certificate-authenticated Java transport, mounted ZIPs, and PostgreSQL.
It admits one caller after the restored version becomes eligible. The exact
result is `{"result":"restored/ping"}`.

## Prepare the disposable cluster

Use the local k3d cluster `loomspan-cloud-pr1-cg1`, with the engine and management
deployments, service account, and transport and Console Secrets from
[the base K3s setup](../README.md). Load the `eclipse-temurin:21-jre` and
`postgres:17-alpine` images into the local Docker daemon. Build the executable JAR
and its test classes with JDK 21, Maven 3.9+, the Framework starter snapshot, and
`mvn verify` in `execution-engine`. The ordinary invocation tests include the
separate `first/ping` proof.

From the repository root, run

```powershell
$kubeconfig = Join-Path $PWD 'execution-engine/target/pr1-k3s/kubeconfig.yaml'
& ./deploy/k3s/milestone/run.ps1 -Kubeconfig $kubeconfig -Context k3d-loomspan-cloud-pr1-cg1
```

Use `-K3dExecutable` if the k3d CLI is installed away from the prepared workspace
or absent from PATH. The runner builds both images, checks the supplied local
context, and uses disposable database and pack volumes. It scales the ordinary
management deployment to zero so the probe is the sole publication writer.
Each run recreates the fixture database, pack volume, and probe Job. The runner
leaves the fixture engine and probe evidence in that context after completion.

## Read the checkpoints

- `K3S_MILESTONE_REPLACE` names the sole old pod, its UID, the old JVM ID, and
  the publication acknowledged by Java and independently reopened from
  PostgreSQL. The probe withholds the resulting Console snapshot. The runner
  verifies the pod identity, scales the engine to zero until that pod is gone,
  then starts its replacement with the same image.
- `K3S_MILESTONE_RESTORED` records the old and new JVM IDs, the durable version,
  the number of invoke dispatches before confirmation, and the first result.
  The probe holds the new JVM's post-restore snapshot before confirming routing.
- `K3S_MILESTONE_DIAGNOSTICS` records the four switch and outcome combinations,
  plus pre-session input validation with an explicit null view. The namespace
  retains trace policy `NEVER` throughout.
