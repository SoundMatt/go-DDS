// Package netpolicy renders a Kubernetes NetworkPolicy from a DDSDomain
// spec, implementing the "domain-per-namespace isolation with network
// policy generation" half of Milestone 15's Kubernetes Operator (see
// ROADMAP.md, "Kubernetes Operator"). Like internal/inject, it is a pure
// function of its inputs — no client, no cluster access — so the policy
// shape is unit-testable directly; internal/controller applies the result.
package netpolicy

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/SoundMatt/go-DDS/k8s/operator/api/v1alpha1"
)

// NamePrefix is prepended to the owning DDSDomain's name to derive the
// generated NetworkPolicy's name, keeping it distinguishable from
// user-authored policies and safely re-appliable (same name -> same
// object identity across reconciles).
const NamePrefix = "go-dds-domain-"

// NamespaceNameLabel is the label Kubernetes >=1.21 automatically sets on
// every Namespace to its own name (docs.k8s.io "namespaces/#automatic-labelling"),
// used here to build namespaceSelector match expressions for
// AllowedNamespaces without requiring callers to pre-label anything.
const NamespaceNameLabel = "kubernetes.io/metadata.name"

// RTPS well-known UDP port formulas from DDSI-RTPS §9.6.1.1: for domain ID
// d and participant index p (0-based, up to ParticipantIndexRange-1):
//
//	SPDP multicast (discovery):     7400 + 250*d
//	SPDP unicast:                   7410 + 250*d + 2*p
//	User-data unicast (best-effort default participant slot):
//	                                 7411 + 250*d + 2*p
//
// The generated policy allows the whole per-domain port band rather than
// enumerating participant indices, since the operator does not control
// how many participants a namespace ends up running.
const (
	rtpsPortBase       = 7400
	rtpsPortsPerDomain = 250
	// ParticipantIndexRange bounds how many participant-index slots (each
	// consuming 2 ports for unicast metatraffic + user traffic) the policy
	// opens per domain; 16 comfortably covers typical per-namespace pod
	// counts while keeping the allowed range bounded.
	ParticipantIndexRange = 16
)

// DomainPortRange returns the inclusive [start, end] UDP port range
// containing every well-known RTPS port (SPDP multicast/unicast and
// default user-data unicast) for the given domain ID, across
// ParticipantIndexRange participant slots.
func DomainPortRange(domainID int) (start, end int) {
	start = rtpsPortBase + rtpsPortsPerDomain*domainID
	end = start + 11 + 2*ParticipantIndexRange // covers up to +11 (7411 offset) plus 2*p growth
	return start, end
}

// Build renders the NetworkPolicy for a DDSDomain named domainName in
// namespace ns. It returns nil when spec.IsolateNamespace is false — the
// controller interprets a nil result as "no policy should exist" and
// deletes any previously generated one.
func Build(ns, domainName string, spec *v1alpha1.DDSDomainSpec) *networkingv1.NetworkPolicy {
	if spec == nil || !spec.IsolateNamespace {
		return nil
	}

	start, end := DomainPortRange(spec.DomainID)
	udp := corev1.ProtocolUDP
	portRange := networkingv1.NetworkPolicyPort{
		Protocol: &udp,
		Port:     ptrIntOrString(intstr.FromInt32(int32(start))),
		EndPort:  ptrInt32(int32(end)),
	}

	peers := []networkingv1.NetworkPolicyPeer{
		// Always allow the domain's own namespace.
		{NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{NamespaceNameLabel: ns},
		}},
	}
	for _, allowed := range spec.AllowedNamespaces {
		peers = append(peers, networkingv1.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{NamespaceNameLabel: allowed},
			},
		})
	}

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NamePrefix + domainName,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "go-dds-operator",
				v1alpha1.DomainLabel:           domainName,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			// Empty PodSelector = applies to every pod in the namespace —
			// the whole point of "domain-per-namespace" isolation.
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				Ports: []networkingv1.NetworkPolicyPort{portRange},
				From:  peers,
			}},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				Ports: []networkingv1.NetworkPolicyPort{portRange},
				To:    peers,
			}},
		},
	}
}

func ptrIntOrString(v intstr.IntOrString) *intstr.IntOrString { return &v }
func ptrInt32(v int32) *int32                                 { return &v }
