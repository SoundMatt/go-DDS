package inject

import (
	"encoding/json"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/SoundMatt/go-DDS/k8s/operator/api/v1alpha1"
)

func TestComputeEnv_Nil(t *testing.T) {
	if got := ComputeEnv(nil, nil); got != nil {
		t.Fatalf("ComputeEnv(nil, nil) = %v, want nil", got)
	}
}

func TestComputeEnv_ParticipantOnly(t *testing.T) {
	p := &v1alpha1.DDSParticipantSpec{
		Domain:     5,
		QoSProfile: "reliable",
		Transport:  v1alpha1.TransportSpec{Kind: "tcp", Port: 7412},
		Peers:      []string{"a:1", "b:2"},
	}
	got := ComputeEnv(p, nil)
	want := []corev1.EnvVar{
		{Name: EnvDomainID, Value: "5"},
		{Name: EnvQoSProfile, Value: "reliable"},
		{Name: EnvTransport, Value: "tcp"},
		{Name: EnvTransportPort, Value: "7412"},
		{Name: EnvPeers, Value: "a:1,b:2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ComputeEnv = %+v, want %+v", got, want)
	}
}

func TestComputeEnv_DomainFallback(t *testing.T) {
	// Participant.Domain is zero (unset) -> falls back to the DDSDomain's
	// DomainID governing the namespace.
	p := &v1alpha1.DDSParticipantSpec{}
	d := &v1alpha1.DDSDomainSpec{DomainID: 9}
	got := ComputeEnv(p, d)
	want := []corev1.EnvVar{{Name: EnvDomainID, Value: "9"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ComputeEnv = %+v, want %+v", got, want)
	}
}

func TestComputeEnv_ParticipantDomainWinsOverDomain(t *testing.T) {
	p := &v1alpha1.DDSParticipantSpec{Domain: 2}
	d := &v1alpha1.DDSDomainSpec{DomainID: 9}
	got := ComputeEnv(p, d)
	if got[0].Value != "2" {
		t.Fatalf("DomainID = %s, want 2 (participant override)", got[0].Value)
	}
}

func TestTargetContainers(t *testing.T) {
	containers := []corev1.Container{{Name: "app"}, {Name: "sidecar"}}

	all := TargetContainers("", containers)
	if len(all) != 2 || !all["app"] || !all["sidecar"] {
		t.Errorf("TargetContainers(\"\") = %v", all)
	}

	explicitAll := TargetContainers("all", containers)
	if len(explicitAll) != 2 {
		t.Errorf("TargetContainers(\"all\") = %v", explicitAll)
	}

	one := TargetContainers("app", containers)
	if len(one) != 1 || !one["app"] {
		t.Errorf("TargetContainers(\"app\") = %v", one)
	}

	spaced := TargetContainers("app, sidecar", containers)
	if len(spaced) != 2 {
		t.Errorf("TargetContainers(\"app, sidecar\") = %v", spaced)
	}
}

func TestBuildPatch_NoEnv(t *testing.T) {
	patch, err := BuildPatch("/spec/containers", []corev1.Container{{Name: "app"}}, map[string]bool{"app": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if patch != nil {
		t.Fatalf("expected nil patch, got %s", patch)
	}
}

func TestBuildPatch_EmptyContainerEnv(t *testing.T) {
	containers := []corev1.Container{{Name: "app"}, {Name: "other"}}
	targets := map[string]bool{"app": true}
	env := []corev1.EnvVar{{Name: EnvDomainID, Value: "1"}}

	patch, err := BuildPatch("/spec/containers", containers, targets, env)
	if err != nil {
		t.Fatal(err)
	}

	var ops []map[string]interface{}
	if err := json.Unmarshal(patch, &ops); err != nil {
		t.Fatalf("invalid JSON patch: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1: %s", len(ops), patch)
	}
	if ops[0]["path"] != "/spec/containers/0/env" || ops[0]["op"] != "add" {
		t.Errorf("op = %+v", ops[0])
	}
}

func TestBuildPatch_AppendsToExistingEnv(t *testing.T) {
	containers := []corev1.Container{
		{Name: "app", Env: []corev1.EnvVar{{Name: "EXISTING", Value: "x"}}},
	}
	targets := map[string]bool{"app": true}
	env := []corev1.EnvVar{{Name: EnvDomainID, Value: "1"}, {Name: EnvQoSProfile, Value: "reliable"}}

	patch, err := BuildPatch("/spec/containers", containers, targets, env)
	if err != nil {
		t.Fatal(err)
	}
	var ops []map[string]interface{}
	if err := json.Unmarshal(patch, &ops); err != nil {
		t.Fatalf("invalid JSON patch: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("got %d ops, want 2 (append-per-var): %s", len(ops), patch)
	}
	for _, op := range ops {
		if op["path"] != "/spec/containers/0/env/-" {
			t.Errorf("path = %v, want append path", op["path"])
		}
	}
}

func TestBuildPatch_SkipsAlreadyDefinedVars(t *testing.T) {
	containers := []corev1.Container{
		{Name: "app", Env: []corev1.EnvVar{{Name: EnvDomainID, Value: "override"}}},
	}
	targets := map[string]bool{"app": true}
	env := []corev1.EnvVar{{Name: EnvDomainID, Value: "1"}}

	patch, err := BuildPatch("/spec/containers", containers, targets, env)
	if err != nil {
		t.Fatal(err)
	}
	if patch != nil {
		t.Fatalf("expected no patch (var already defined by user), got %s", patch)
	}
}

func TestBuildPatch_SkipsUntargetedContainers(t *testing.T) {
	containers := []corev1.Container{{Name: "app"}, {Name: "istio-proxy"}}
	targets := map[string]bool{"app": true}
	env := []corev1.EnvVar{{Name: EnvDomainID, Value: "1"}}

	patch, err := BuildPatch("/spec/containers", containers, targets, env)
	if err != nil {
		t.Fatal(err)
	}
	var ops []map[string]interface{}
	if err := json.Unmarshal(patch, &ops); err != nil {
		t.Fatalf("invalid JSON patch: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1 (only 'app' targeted)", len(ops))
	}
	if ops[0]["path"] != "/spec/containers/0/env" {
		t.Errorf("path = %v, want index 0 (app)", ops[0]["path"])
	}
}
