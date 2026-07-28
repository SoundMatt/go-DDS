// Package webhook implements the mutating admission webhook that answers
// "operator discovers participants in annotated pods and injects domain/peer
// config via env" (ROADMAP.md, Milestone 15, "Kubernetes Operator"). It
// receives AdmissionReview requests for Pod create (and update, for
// completeness) operations, resolves the pod's ParticipantAnnotation and
// namespace-bound DDSDomain against the caches populated by
// internal/controller, and returns a JSON Patch built by internal/inject.
package webhook

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/SoundMatt/go-DDS/k8s/operator/api/v1alpha1"
	"github.com/SoundMatt/go-DDS/k8s/operator/internal/inject"
)

// ParticipantLookup resolves a DDSParticipant's spec by namespace/name.
// Implemented by internal/cache.Participants.
type ParticipantLookup interface {
	Get(namespace, name string) (*v1alpha1.DDSParticipantSpec, bool)
}

// DomainLookup resolves a DDSDomain's spec by namespace/name. Implemented
// by internal/cache.Domains.
type DomainLookup interface {
	Get(namespace, name string) (*v1alpha1.DDSDomainSpec, bool)
}

// NamespaceDomainLookup resolves which DDSDomain (by name, in the same
// namespace) governs a namespace. Implemented by
// internal/cache.NamespaceDomains.
type NamespaceDomainLookup interface {
	Get(namespace string) (domainName string, ok bool)
}

// Mutator answers the /mutate-pods AdmissionReview request.
type Mutator struct {
	Participants ParticipantLookup
	Domains      DomainLookup
	NamespaceMap NamespaceDomainLookup
	Log          *slog.Logger
}

func (m *Mutator) logger() *slog.Logger {
	if m.Log != nil {
		return m.Log
	}
	return slog.Default()
}

// ServeHTTP implements the standard Kubernetes admission-webhook HTTP
// contract: POST a v1 AdmissionReview, respond with a v1 AdmissionReview
// carrying an AdmissionResponse.
func (m *Mutator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20)) // 5 MiB is generous for a Pod object
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(body, &review); err != nil {
		http.Error(w, "invalid AdmissionReview: "+err.Error(), http.StatusBadRequest)
		return
	}
	if review.Request == nil {
		http.Error(w, "AdmissionReview.request is nil", http.StatusBadRequest)
		return
	}

	resp := m.review(review.Request)
	out := admissionv1.AdmissionReview{
		TypeMeta: review.TypeMeta,
		Response: resp,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		m.logger().Error("webhook: failed to encode AdmissionReview response", "error", err)
	}
}

// review computes the AdmissionResponse for a single AdmissionRequest. It
// always allows the pod through (fail-open): a missing or not-yet-synced
// DDSParticipant means "nothing to inject", surfaced as a non-blocking
// warning rather than a denial, because a busy or momentarily-behind
// operator must never be able to take workloads outside its own domain
// down with it. Only a malformed request body is rejected — everything
// past `json.Unmarshal` on the embedded Pod always allows.
func (m *Mutator) review(req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	allow := func(warnings ...string) *admissionv1.AdmissionResponse {
		return &admissionv1.AdmissionResponse{UID: req.UID, Allowed: true, Warnings: warnings}
	}

	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		return allow("go-dds-operator: could not decode Pod, skipping injection: " + err.Error())
	}

	participantName := pod.Annotations[v1alpha1.ParticipantAnnotation]
	if participantName == "" {
		return allow()
	}

	namespace := podNamespace(&pod, req.Namespace)

	participant, ok := m.Participants.Get(namespace, participantName)
	if !ok {
		return allow("go-dds-operator: DDSParticipant " + namespace + "/" + participantName + " not found, skipping injection")
	}

	var domain *v1alpha1.DDSDomainSpec
	if m.NamespaceMap != nil && m.Domains != nil {
		if domainName, ok := m.NamespaceMap.Get(namespace); ok {
			domain, _ = m.Domains.Get(namespace, domainName)
		}
	}

	env := inject.ComputeEnv(participant, domain)
	targets := inject.TargetContainers(pod.Annotations[v1alpha1.InjectContainersAnnotation], pod.Spec.Containers)
	patch, err := inject.BuildPatch("/spec/containers", pod.Spec.Containers, targets, env)
	if err != nil {
		return allow("go-dds-operator: failed to build injection patch: " + err.Error())
	}
	if patch == nil {
		return allow()
	}

	patchType := admissionv1.PatchTypeJSONPatch
	return &admissionv1.AdmissionResponse{
		UID:       req.UID,
		Allowed:   true,
		Patch:     patch,
		PatchType: &patchType,
	}
}

// podNamespace returns the pod's own namespace if set (rare at admission
// time — namespace is usually only carried on the AdmissionRequest), else
// falls back to the request's namespace.
func podNamespace(pod *corev1.Pod, reqNamespace string) string {
	if pod.Namespace != "" {
		return pod.Namespace
	}
	return reqNamespace
}
