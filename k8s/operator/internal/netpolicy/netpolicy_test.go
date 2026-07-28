package netpolicy

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/SoundMatt/go-DDS/k8s/operator/api/v1alpha1"
)

func TestBuild_NotIsolated(t *testing.T) {
	if got := Build("ns", "d1", &v1alpha1.DDSDomainSpec{DomainID: 1, IsolateNamespace: false}); got != nil {
		t.Fatalf("Build() with IsolateNamespace=false = %+v, want nil", got)
	}
	if got := Build("ns", "d1", nil); got != nil {
		t.Fatalf("Build(nil spec) = %+v, want nil", got)
	}
}

func TestBuild_Basic(t *testing.T) {
	np := Build("telemetry", "prod", &v1alpha1.DDSDomainSpec{DomainID: 2, IsolateNamespace: true})
	if np == nil {
		t.Fatal("Build() = nil, want a NetworkPolicy")
	}
	if np.Name != "go-dds-domain-prod" {
		t.Errorf("Name = %q", np.Name)
	}
	if np.Namespace != "telemetry" {
		t.Errorf("Namespace = %q", np.Namespace)
	}
	if len(np.Spec.PolicyTypes) != 2 {
		t.Errorf("PolicyTypes = %v, want Ingress+Egress", np.Spec.PolicyTypes)
	}
	if len(np.Spec.Ingress) != 1 || len(np.Spec.Ingress[0].From) != 1 {
		t.Fatalf("Ingress = %+v", np.Spec.Ingress)
	}
	if np.Spec.Ingress[0].From[0].NamespaceSelector.MatchLabels[NamespaceNameLabel] != "telemetry" {
		t.Errorf("ingress peer = %+v", np.Spec.Ingress[0].From[0])
	}
	if len(np.Spec.Egress) != 1 || len(np.Spec.Egress[0].To) != 1 {
		t.Fatalf("Egress = %+v", np.Spec.Egress)
	}

	port := np.Spec.Ingress[0].Ports[0]
	if *port.Protocol != corev1.ProtocolUDP {
		t.Errorf("Protocol = %v, want UDP", *port.Protocol)
	}
	wantStart, wantEnd := DomainPortRange(2)
	if port.Port.IntValue() != wantStart {
		t.Errorf("Port = %d, want %d", port.Port.IntValue(), wantStart)
	}
	if port.EndPort == nil || int(*port.EndPort) != wantEnd {
		t.Errorf("EndPort = %v, want %d", port.EndPort, wantEnd)
	}
}

func TestBuild_AllowedNamespaces(t *testing.T) {
	np := Build("prod", "d", &v1alpha1.DDSDomainSpec{
		DomainID:          0,
		IsolateNamespace:  true,
		AllowedNamespaces: []string{"staging", "shared"},
	})
	if len(np.Spec.Ingress[0].From) != 3 { // own namespace + 2 allowed
		t.Fatalf("From = %+v, want 3 peers", np.Spec.Ingress[0].From)
	}
	names := map[string]bool{}
	for _, p := range np.Spec.Ingress[0].From {
		names[p.NamespaceSelector.MatchLabels[NamespaceNameLabel]] = true
	}
	for _, want := range []string{"prod", "staging", "shared"} {
		if !names[want] {
			t.Errorf("missing peer namespace %q in %v", want, names)
		}
	}
}

func TestDomainPortRange_Monotonic(t *testing.T) {
	s0, e0 := DomainPortRange(0)
	s1, e1 := DomainPortRange(1)
	if s1 <= s0 || e1 <= e0 {
		t.Errorf("DomainPortRange must grow with domain ID: (%d,%d) then (%d,%d)", s0, e0, s1, e1)
	}
	if s0 != rtpsPortBase {
		t.Errorf("DomainPortRange(0) start = %d, want %d", s0, rtpsPortBase)
	}
}

func TestBuild_PodSelectorAppliesNamespaceWide(t *testing.T) {
	np := Build("ns", "d", &v1alpha1.DDSDomainSpec{DomainID: 1, IsolateNamespace: true})
	empty := networkingv1.NetworkPolicy{}.Spec.PodSelector
	if len(np.Spec.PodSelector.MatchLabels) != len(empty.MatchLabels) {
		t.Errorf("PodSelector should be empty (applies to all pods), got %+v", np.Spec.PodSelector)
	}
}
