package cache

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/SoundMatt/go-DDS/k8s/operator/api/v1alpha1"
)

func newParticipantObj(ns, name string, domain int) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": v1alpha1.Group + "/" + v1alpha1.Version,
		"kind":       "DDSParticipant",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		"spec": map[string]interface{}{
			"domain": int64(domain),
		},
	}}
}

// TestParticipants_InformerSync exercises the full path: a fake dynamic
// client seeded with one DDSParticipant, a real dynamicinformer factory,
// and Participants.EventHandler wired to it — proving the cache converges
// to the informer's initial list without a live cluster.
func TestParticipants_InformerSync(t *testing.T) {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		v1alpha1.ParticipantGVR: "DDSParticipantList",
	}
	seed := newParticipantObj("default", "sensor-pub", 4)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, seed)

	factory := dynamicinformer.NewDynamicSharedInformerFactory(client, 0)
	informer := factory.ForResource(v1alpha1.ParticipantGVR).Informer()

	participants := NewParticipants()
	if _, err := informer.AddEventHandler(participants.EventHandler()); err != nil {
		t.Fatalf("AddEventHandler: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	factory.Start(ctx.Done())
	if !waitForSync(ctx, factory) {
		t.Fatal("informer cache never synced")
	}

	spec, ok := participants.Get("default", "sensor-pub")
	if !ok {
		t.Fatal("expected cached participant, got none")
	}
	if spec.Domain != 4 {
		t.Errorf("Domain = %d, want 4", spec.Domain)
	}
	if participants.Len() != 1 {
		t.Errorf("Len() = %d, want 1", participants.Len())
	}

	// Delete via the fake client and confirm the cache observes it.
	if err := client.Resource(v1alpha1.ParticipantGVR).Namespace("default").
		Delete(ctx, "sensor-pub", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !waitFor(func() bool { _, ok := participants.Get("default", "sensor-pub"); return !ok }) {
		t.Fatal("cache still has participant after delete")
	}
}

func TestDomains_InformerSync(t *testing.T) {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		v1alpha1.DomainGVR: "DDSDomainList",
	}
	seed := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": v1alpha1.Group + "/" + v1alpha1.Version,
		"kind":       "DDSDomain",
		"metadata": map[string]interface{}{
			"name":      "prod",
			"namespace": "telemetry",
		},
		"spec": map[string]interface{}{
			"domainID":         int64(2),
			"isolateNamespace": true,
		},
	}}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, seed)

	factory := dynamicinformer.NewDynamicSharedInformerFactory(client, 0)
	informer := factory.ForResource(v1alpha1.DomainGVR).Informer()

	domains := NewDomains()
	if _, err := informer.AddEventHandler(domains.EventHandler()); err != nil {
		t.Fatalf("AddEventHandler: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	factory.Start(ctx.Done())
	if !waitForSync(ctx, factory) {
		t.Fatal("informer cache never synced")
	}

	spec, ok := domains.Get("telemetry", "prod")
	if !ok {
		t.Fatal("expected cached domain, got none")
	}
	if spec.DomainID != 2 || !spec.IsolateNamespace {
		t.Errorf("spec = %+v", spec)
	}
}

func TestNamespaceDomains_Label(t *testing.T) {
	nd := NewNamespaceDomains()
	ns := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":   "telemetry",
			"labels": map[string]interface{}{v1alpha1.DomainLabel: "prod"},
		},
	}}
	nd.upsert(ns)
	got, ok := nd.Get("telemetry")
	if !ok || got != "prod" {
		t.Fatalf("Get(telemetry) = (%q, %v), want (prod, true)", got, ok)
	}

	// Removing the label should clear the binding on the next update.
	unlabeled := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "telemetry"},
	}}
	nd.upsert(unlabeled)
	if _, ok := nd.Get("telemetry"); ok {
		t.Fatal("expected binding cleared after label removal")
	}
}

func TestToUnstructured_IgnoresUnknownTypes(t *testing.T) {
	if _, ok := toUnstructured("not an unstructured object"); ok {
		t.Fatal("expected ok=false for a non-Unstructured object")
	}
}

func waitForSync(ctx context.Context, factory dynamicinformer.DynamicSharedInformerFactory) bool {
	done := make(chan struct{})
	var ok bool
	go func() {
		for _, synced := range factory.WaitForCacheSync(ctx.Done()) {
			ok = synced
			if !ok {
				break
			}
		}
		close(done)
	}()
	select {
	case <-done:
		return ok
	case <-ctx.Done():
		return false
	}
}

// waitFor polls cond until it returns true or a short deadline passes,
// giving the fake client's watch machinery time to deliver the delete
// event to the informer.
func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}
