package v1alpha1

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestParticipantSpecFromUnstructured(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": Group + "/" + Version,
		"kind":       "DDSParticipant",
		"metadata": map[string]interface{}{
			"name":      "sensor-pub",
			"namespace": "default",
		},
		"spec": map[string]interface{}{
			"domain":     int64(3),
			"qosProfile": "reliable-transient-local",
			"transport": map[string]interface{}{
				"kind": "tcp",
				"port": int64(7412),
			},
			"peers": []interface{}{"10.0.0.1:7400", "10.0.0.2:7400"},
		},
	}}

	spec, err := ParticipantSpecFromUnstructured(u)
	if err != nil {
		t.Fatalf("ParticipantSpecFromUnstructured: %v", err)
	}
	if spec.Domain != 3 {
		t.Errorf("Domain = %d, want 3", spec.Domain)
	}
	if spec.QoSProfile != "reliable-transient-local" {
		t.Errorf("QoSProfile = %q", spec.QoSProfile)
	}
	if spec.Transport.Kind != "tcp" || spec.Transport.Port != 7412 {
		t.Errorf("Transport = %+v", spec.Transport)
	}
	if len(spec.Peers) != 2 || spec.Peers[0] != "10.0.0.1:7400" {
		t.Errorf("Peers = %v", spec.Peers)
	}
}

func TestParticipantSpecFromUnstructured_NoSpec(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "x", "namespace": "ns"},
	}}
	if _, err := ParticipantSpecFromUnstructured(u); err == nil {
		t.Fatal("expected error for missing spec, got nil")
	}
}

func TestDomainSpecFromUnstructured(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"domainID":          int64(7),
			"isolateNamespace":  true,
			"allowedNamespaces": []interface{}{"telemetry"},
		},
	}}

	spec, err := DomainSpecFromUnstructured(u)
	if err != nil {
		t.Fatalf("DomainSpecFromUnstructured: %v", err)
	}
	if spec.DomainID != 7 {
		t.Errorf("DomainID = %d, want 7", spec.DomainID)
	}
	if !spec.IsolateNamespace {
		t.Error("IsolateNamespace = false, want true")
	}
	if len(spec.AllowedNamespaces) != 1 || spec.AllowedNamespaces[0] != "telemetry" {
		t.Errorf("AllowedNamespaces = %v", spec.AllowedNamespaces)
	}
}

func TestDomainSpecFromUnstructured_NoSpec(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	if _, err := DomainSpecFromUnstructured(u); err == nil {
		t.Fatal("expected error for missing spec, got nil")
	}
}

func TestGVRs(t *testing.T) {
	if ParticipantGVR.Resource != "ddsparticipants" || ParticipantGVR.Group != Group || ParticipantGVR.Version != Version {
		t.Errorf("ParticipantGVR = %+v", ParticipantGVR)
	}
	if DomainGVR.Resource != "ddsdomains" || DomainGVR.Group != Group || DomainGVR.Version != Version {
		t.Errorf("DomainGVR = %+v", DomainGVR)
	}
}
