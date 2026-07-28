# go-dds-operator

The go-DDS Kubernetes operator (ROADMAP.md, Milestone 15 "Cloud-Native
Runtime", "Kubernetes Operator"). It is a separate Go module
(`github.com/SoundMatt/go-DDS/k8s/operator`) with its own `go.mod`, per the
"Architecture Initiative" multi-module convention — see the repo root
`ROADMAP.md` and `go.work`.

It ships three pieces:

1. Two CRDs — `DDSParticipant` and `DDSDomain` (`dds.soundmatt.io/v1alpha1`,
   `config/crd/*.yaml`).
2. A mutating admission webhook that discovers pods annotated
   `dds.soundmatt.io/participant: <name>` and injects `DDS_*` environment
   variables resolved from the matching `DDSParticipant` (and, if the pod's
   namespace is labeled `dds.soundmatt.io/domain: <name>`, the matching
   `DDSDomain`) into the pod's containers at admission time.
3. A controller that reconciles `DDSDomain` objects with
   `spec.isolateNamespace: true` into a `NetworkPolicy` scoping that
   domain's RTPS UDP port range to the owning namespace (plus
   `spec.allowedNamespaces`).

## Why no code-generated clientset

The two CRDs are read via the dynamic client as `unstructured.Unstructured`
and converted to/from the plain Go structs in `api/v1alpha1` with
`runtime.DefaultUnstructuredConverter`, rather than through a
`k8s.io/code-generator`-generated typed clientset/informers/deepcopy. That
generator needs a full Kubernetes source checkout and a Bash/Make toolchain
this repo doesn't otherwise depend on. This keeps the operator buildable
with nothing but `go build`, in the same "pure Go first" spirit as the
hand-rolled Prometheus exposition format in
`observability/monitor/prometheus.go` — see `api/v1alpha1`'s package doc.

## Layout

```
api/v1alpha1/         DDSParticipantSpec, DDSDomainSpec + unstructured<->struct conversion
internal/inject/       Pure env-var + JSON Patch computation (no cluster access)
internal/netpolicy/    Pure DDSDomain -> NetworkPolicy rendering (no cluster access)
internal/cache/        Informer-backed lookup caches the webhook reads synchronously
internal/webhook/       AdmissionReview HTTP handler wiring inject+cache together
internal/controller/    DDSDomain -> NetworkPolicy reconcile loop
cmd/operator/           main: wires clients, informers, webhook + health servers
config/crd/             CRD manifests (kubectl apply -f)
config/samples/         A complete worked example
deploy/helm/go-dds-operator/  Helm chart (CRDs + RBAC + Deployment + webhook + self-signed TLS)
```

## Quickstart

```sh
helm install go-dds-operator ./deploy/helm/go-dds-operator -n go-dds-system --create-namespace
kubectl apply -f config/samples/full-example.yaml
```

See `deploy/helm/go-dds-operator/templates/NOTES.txt` (also printed by
`helm install`) for the annotation/label contract, and
`config/samples/full-example.yaml` for a complete namespace + DDSDomain +
DDSParticipant + annotated Deployment.

## Design notes

- **Fail-open injection.** A pod referencing a `DDSParticipant` that
  doesn't exist (yet, or a typo) is still admitted — with a warning, not a
  denial. See `internal/webhook`'s package doc: an operator hiccup or cache
  lag must never be able to block unrelated workloads from scheduling. The
  Helm chart's default `webhook.failurePolicy: Ignore` reinforces this at
  the Kubernetes level too.
- **Explicit config always wins.** `BuildPatch` (`internal/inject`) never
  overwrites a container env var the pod spec already defines, and a
  `DDSParticipant.spec.domain` always wins over its namespace's
  `DDSDomain.spec.domainID` — the CRD is a default, not an override.
- **NetworkPolicy port range.** `internal/netpolicy.DomainPortRange`
  derives the generated policy's UDP port range from the RTPS well-known
  port formulas (DDSI-RTPS §9.6.1.1) for the domain's SPDP
  multicast/unicast and default user-data unicast traffic, across a bounded
  number of per-namespace participant-index slots
  (`netpolicy.ParticipantIndexRange`).
- **Self-signed TLS by default.** The Helm chart generates the webhook's
  serving certificate at install/upgrade time via Helm's `genCA`/
  `genSignedCert` — zero extra tooling (no cert-manager dependency), at the
  cost of a new certificate (and brief webhook Pod restart) on every `helm
  upgrade`. Set `tls.selfSigned: false` with `tls.certSecretName` /
  `tls.certManagerCertificateName` for cert-manager-issued certificates in
  production. See `values.yaml`.
