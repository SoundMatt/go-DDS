package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/SoundMatt/go-DDS/k8s/operator/api/v1alpha1"
)

type fakeParticipants map[string]*v1alpha1.DDSParticipantSpec

func (f fakeParticipants) Get(ns, name string) (*v1alpha1.DDSParticipantSpec, bool) {
	v, ok := f[ns+"/"+name]
	return v, ok
}

type fakeDomains map[string]*v1alpha1.DDSDomainSpec

func (f fakeDomains) Get(ns, name string) (*v1alpha1.DDSDomainSpec, bool) {
	v, ok := f[ns+"/"+name]
	return v, ok
}

type fakeNamespaceMap map[string]string

func (f fakeNamespaceMap) Get(ns string) (string, bool) {
	v, ok := f[ns]
	return v, ok
}

func admissionReviewFor(t *testing.T, pod *corev1.Pod, namespace string) *admissionv1.AdmissionReview {
	t.Helper()
	raw, err := json.Marshal(pod)
	if err != nil {
		t.Fatalf("marshal pod: %v", err)
	}
	return &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request: &admissionv1.AdmissionRequest{
			UID:       types.UID("test-uid"),
			Namespace: namespace,
			Object:    runtime.RawExtension{Raw: raw},
		},
	}
}

func postReview(t *testing.T, m *Mutator, review *admissionv1.AdmissionReview) *admissionv1.AdmissionReview {
	t.Helper()
	body, err := json.Marshal(review)
	if err != nil {
		t.Fatalf("marshal review: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mutate-pods", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out admissionv1.AdmissionReview
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v; body=%s", err, rec.Body.String())
	}
	return &out
}

func TestServeHTTP_NoAnnotation_Allowed_NoPatch(t *testing.T) {
	m := &Mutator{Participants: fakeParticipants{}}
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}}
	out := postReview(t, m, admissionReviewFor(t, pod, "default"))

	if !out.Response.Allowed {
		t.Fatal("expected Allowed=true")
	}
	if out.Response.Patch != nil {
		t.Fatalf("expected no patch, got %s", out.Response.Patch)
	}
	if out.Response.UID != "test-uid" {
		t.Errorf("UID = %q", out.Response.UID)
	}
}

func TestServeHTTP_ParticipantNotFound_AllowedWithWarning(t *testing.T) {
	m := &Mutator{Participants: fakeParticipants{}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{v1alpha1.ParticipantAnnotation: "ghost"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	out := postReview(t, m, admissionReviewFor(t, pod, "default"))

	if !out.Response.Allowed {
		t.Fatal("expected Allowed=true (fail-open)")
	}
	if out.Response.Patch != nil {
		t.Fatal("expected no patch when participant is missing")
	}
	if len(out.Response.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %v", out.Response.Warnings)
	}
}

func TestServeHTTP_InjectsEnv(t *testing.T) {
	participants := fakeParticipants{
		"default/sensor-pub": {Domain: 3, QoSProfile: "reliable"},
	}
	m := &Mutator{Participants: participants}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{v1alpha1.ParticipantAnnotation: "sensor-pub"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	out := postReview(t, m, admissionReviewFor(t, pod, "default"))

	if !out.Response.Allowed {
		t.Fatal("expected Allowed=true")
	}
	if out.Response.Patch == nil {
		t.Fatal("expected a patch")
	}
	if out.Response.PatchType == nil || *out.Response.PatchType != admissionv1.PatchTypeJSONPatch {
		t.Fatalf("PatchType = %v, want JSONPatch", out.Response.PatchType)
	}

	var ops []map[string]interface{}
	if err := json.Unmarshal(out.Response.Patch, &ops); err != nil {
		t.Fatalf("invalid patch JSON: %v", err)
	}
	if len(ops) != 1 || ops[0]["path"] != "/spec/containers/0/env" {
		t.Fatalf("ops = %+v", ops)
	}
}

func TestServeHTTP_DomainFallbackViaNamespace(t *testing.T) {
	participants := fakeParticipants{"telemetry/edge": {}} // Domain unset -> falls back to namespace's DDSDomain
	domains := fakeDomains{"telemetry/prod": {DomainID: 9}}
	nsMap := fakeNamespaceMap{"telemetry": "prod"}

	m := &Mutator{Participants: participants, Domains: domains, NamespaceMap: nsMap}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{v1alpha1.ParticipantAnnotation: "edge"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	out := postReview(t, m, admissionReviewFor(t, pod, "telemetry"))

	var ops []struct {
		Value []corev1.EnvVar `json:"value"`
	}
	if err := json.Unmarshal(out.Response.Patch, &ops); err != nil {
		t.Fatalf("invalid patch JSON: %v; patch=%s", err, out.Response.Patch)
	}
	if len(ops) != 1 || len(ops[0].Value) != 1 || ops[0].Value[0].Value != "9" {
		t.Fatalf("expected DDS_DOMAIN_ID=9 from namespace-bound DDSDomain, got %+v", ops)
	}
}

func TestServeHTTP_RejectsMalformedBody(t *testing.T) {
	m := &Mutator{Participants: fakeParticipants{}}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mutate-pods", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestServeHTTP_RejectsMissingRequest(t *testing.T) {
	m := &Mutator{Participants: fakeParticipants{}}
	review := admissionv1.AdmissionReview{TypeMeta: metav1.TypeMeta{Kind: "AdmissionReview"}}
	body, _ := json.Marshal(review)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mutate-pods", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestServeHTTP_RejectsNonPost(t *testing.T) {
	m := &Mutator{Participants: fakeParticipants{}}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/mutate-pods", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestServeHTTP_MalformedPodObject_AllowedWithWarning(t *testing.T) {
	m := &Mutator{Participants: fakeParticipants{}}
	review := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{Kind: "AdmissionReview"},
		Request: &admissionv1.AdmissionRequest{
			UID:       types.UID("x"),
			Namespace: "default",
			Object:    runtime.RawExtension{Raw: []byte(`{"spec": "not-an-object"}`)},
		},
	}
	out := postReview(t, m, review)
	if !out.Response.Allowed {
		t.Fatal("expected fail-open Allowed=true for undecodable pod")
	}
	if len(out.Response.Warnings) != 1 {
		t.Fatalf("expected a warning, got %v", out.Response.Warnings)
	}
}
