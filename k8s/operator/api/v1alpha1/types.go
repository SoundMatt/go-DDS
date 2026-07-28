// Package v1alpha1 defines the Go-side shapes of the go-DDS Kubernetes
// operator's two custom resources (ROADMAP.md, Milestone 15, "Kubernetes
// Operator"):
//
//   - DDSParticipant — declarative participant config (domain, QoS profile,
//     transport) that the admission webhook resolves into environment
//     variables injected into annotated pods.
//   - DDSDomain — domain-per-namespace isolation; the controller renders a
//     matching NetworkPolicy from it.
//
// The operator deliberately does not register these as typed
// `runtime.Object` API types wired through client-go's generated-clientset
// machinery (that requires `k8s.io/code-generator`, which needs network
// access to a Kubernetes checkout and a Bazel/Make toolchain this repo does
// not otherwise depend on). Instead, custom resources are read from the
// dynamic client as `unstructured.Unstructured` and converted to/from these
// plain structs with `runtime.DefaultUnstructuredConverter` — the same
// "pure Go first" spirit as the hand-rolled Prometheus exposition in
// observability/monitor/prometheus.go (no generated glue, one obvious
// source of truth for the schema: the `json` struct tags below, mirrored by
// the OpenAPI schema in config/crd/*.yaml).
package v1alpha1

// Group is the API group both CRDs are served under.
const Group = "dds.soundmatt.io"

// Version is the (only, for now) served API version.
const Version = "v1alpha1"

// Resource plural names, as used in the CRD manifests and the dynamic
// client's GroupVersionResource.
const (
	ParticipantResource = "ddsparticipants"
	DomainResource      = "ddsdomains"
)

// Well-known pod annotation and namespace label keys the webhook and
// controllers key discovery off of.
const (
	// ParticipantAnnotation, set on a Pod's template, names the
	// DDSParticipant (in the pod's own namespace) whose config should be
	// injected as environment variables.
	ParticipantAnnotation = "dds.soundmatt.io/participant"

	// InjectContainersAnnotation optionally restricts injection to a
	// comma-separated list of container names; "all" (the default when the
	// annotation is absent) injects into every container.
	InjectContainersAnnotation = "dds.soundmatt.io/inject-containers"

	// DomainLabel, set on a Namespace, names the DDSDomain that namespace
	// belongs to for network-policy generation and default domain-ID
	// injection.
	DomainLabel = "dds.soundmatt.io/domain"
)

// TransportSpec describes which go-DDS transport a participant should use.
type TransportSpec struct {
	// Kind is one of "udp" (default), "tcp", "dtls", or "shmem" — matching
	// the transport names already used across dds.NewParticipant's options
	// and RTPS-over-TCP/DTLS (ROADMAP.md Milestone 14).
	Kind string `json:"kind,omitempty"`

	// Port, when set, overrides the transport's default port.
	Port int `json:"port,omitempty"`
}

// DDSParticipantSpec is the spec of a DDSParticipant custom resource.
type DDSParticipantSpec struct {
	// Domain is the RTPS domain ID (0-232, per DDSI-RTPS §9.6.2.3).
	Domain int `json:"domain"`

	// QoSProfile names a QoS profile understood by the application (opaque
	// to the operator — passed through verbatim as DDS_QOS_PROFILE).
	QoSProfile string `json:"qosProfile,omitempty"`

	// Transport configures the wire transport.
	Transport TransportSpec `json:"transport,omitempty"`

	// Peers is a static peer list (host:port entries) for discovery in
	// environments without multicast, injected as a comma-joined
	// DDS_PEERS env var.
	Peers []string `json:"peers,omitempty"`
}

// DDSDomainSpec is the spec of a DDSDomain custom resource.
type DDSDomainSpec struct {
	// DomainID is the RTPS domain ID this Kubernetes namespace maps to.
	DomainID int `json:"domainID"`

	// IsolateNamespace, when true, makes the controller generate a
	// NetworkPolicy restricting DDS traffic (RTPS metatraffic and
	// user-traffic UDP ports for DomainID, per DDSI-RTPS §9.6.1.1) to pods
	// within the same namespace plus AllowedNamespaces.
	IsolateNamespace bool `json:"isolateNamespace,omitempty"`

	// AllowedNamespaces lists additional namespaces (matched via the
	// standard "kubernetes.io/metadata.name" label) allowed to exchange DDS
	// traffic with this domain's namespace when IsolateNamespace is true.
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`
}
