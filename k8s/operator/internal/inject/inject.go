// Package inject computes the environment-variable configuration a
// DDSParticipant (and, optionally, its namespace's DDSDomain) resolves to,
// and the JSON Patch that adds it to a Pod spec. It is deliberately pure —
// no Kubernetes client, no I/O — so the admission-control decision (what to
// patch) is unit-testable without a cluster; internal/webhook wires it to
// an actual AdmissionReview request/response.
package inject

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/SoundMatt/go-DDS/k8s/operator/api/v1alpha1"
)

// Environment variable names injected into matched containers.
const (
	EnvDomainID      = "DDS_DOMAIN_ID"
	EnvQoSProfile    = "DDS_QOS_PROFILE"
	EnvTransport     = "DDS_TRANSPORT"
	EnvTransportPort = "DDS_TRANSPORT_PORT"
	EnvPeers         = "DDS_PEERS"
)

// ComputeEnv derives the DDS_* environment variables for a pod from its
// matched DDSParticipant (required — nil means "nothing to inject") and,
// optionally, the DDSDomain governing its namespace. The participant's own
// Domain always wins over the DDSDomain's DomainID: an explicit
// DDSParticipant.spec.domain is a deliberate per-workload override, while
// the DDSDomain only supplies a namespace-wide default (used when
// participant.Domain is unset, i.e. zero, and a domain is bound).
//
// Order is deterministic (participant fields first, in struct-field order,
// then peers) so BuildPatch's output — and therefore this whole webhook —
// is reproducible across calls, which matters both for testing and because
// Kubernetes may retry a mutating webhook call with a byte-identical
// request.
func ComputeEnv(participant *v1alpha1.DDSParticipantSpec, domain *v1alpha1.DDSDomainSpec) []corev1.EnvVar {
	if participant == nil {
		return nil
	}

	domainID := participant.Domain
	if domainID == 0 && domain != nil {
		domainID = domain.DomainID
	}

	env := []corev1.EnvVar{
		{Name: EnvDomainID, Value: strconv.Itoa(domainID)},
	}
	if participant.QoSProfile != "" {
		env = append(env, corev1.EnvVar{Name: EnvQoSProfile, Value: participant.QoSProfile})
	}
	if participant.Transport.Kind != "" {
		env = append(env, corev1.EnvVar{Name: EnvTransport, Value: participant.Transport.Kind})
	}
	if participant.Transport.Port != 0 {
		env = append(env, corev1.EnvVar{Name: EnvTransportPort, Value: strconv.Itoa(participant.Transport.Port)})
	}
	if len(participant.Peers) > 0 {
		env = append(env, corev1.EnvVar{Name: EnvPeers, Value: strings.Join(participant.Peers, ",")})
	}
	return env
}

// TargetContainers resolves the InjectContainersAnnotation value ("" or
// "all" means every container) to the set of container names env should be
// injected into.
func TargetContainers(annotationValue string, containers []corev1.Container) map[string]bool {
	targets := make(map[string]bool, len(containers))
	if annotationValue == "" || annotationValue == "all" {
		for _, c := range containers {
			targets[c.Name] = true
		}
		return targets
	}
	for _, name := range strings.Split(annotationValue, ",") {
		if name = strings.TrimSpace(name); name != "" {
			targets[name] = true
		}
	}
	return targets
}

// jsonPatchOp is a single RFC 6902 JSON Patch operation.
type jsonPatchOp struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

// BuildPatch returns the RFC 6902 JSON Patch document (marshaled JSON) that
// adds env to every container in containers whose name is in targets, and
// which does not already define an env var of the same name (an explicit
// container env var always wins over injection — the operator never
// overrides application-declared configuration). containerPaths is the
// path prefix pattern; index i's container lives at
// fmt.Sprintf(pathPrefix+"/%d/env", i).
//
// Returns (nil, nil) — not an error — when there is nothing to patch, so
// callers can distinguish "no patch needed" from a real failure.
func BuildPatch(pathPrefix string, containers []corev1.Container, targets map[string]bool, env []corev1.EnvVar) ([]byte, error) {
	if len(env) == 0 {
		return nil, nil
	}

	var ops []jsonPatchOp
	for i, c := range containers {
		if !targets[c.Name] {
			continue
		}
		existing := make(map[string]bool, len(c.Env))
		for _, e := range c.Env {
			existing[e.Name] = true
		}

		toAdd := make([]corev1.EnvVar, 0, len(env))
		for _, e := range env {
			if !existing[e.Name] {
				toAdd = append(toAdd, e)
			}
		}
		if len(toAdd) == 0 {
			continue
		}

		if len(c.Env) == 0 {
			ops = append(ops, jsonPatchOp{
				Op:    "add",
				Path:  fmt.Sprintf("%s/%d/env", pathPrefix, i),
				Value: toAdd,
			})
			continue
		}
		// Container already has env vars: append each new one individually
		// with "-" (append-to-end) so we never clobber existing entries or
		// need to know their current count against a concurrent mutation.
		for _, e := range toAdd {
			ops = append(ops, jsonPatchOp{
				Op:    "add",
				Path:  fmt.Sprintf("%s/%d/env/-", pathPrefix, i),
				Value: e,
			})
		}
	}
	if len(ops) == 0 {
		return nil, nil
	}
	return json.Marshal(ops)
}
