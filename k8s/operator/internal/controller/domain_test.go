package controller

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/SoundMatt/go-DDS/k8s/operator/api/v1alpha1"
	"github.com/SoundMatt/go-DDS/k8s/operator/internal/netpolicy"
)

func domainObj(ns, name string, domainID int, isolate bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": v1alpha1.Group + "/" + v1alpha1.Version,
		"kind":       "DDSDomain",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		"spec": map[string]interface{}{
			"domainID":         int64(domainID),
			"isolateNamespace": isolate,
		},
	}}
}

func newTestReconciler(t *testing.T, objs ...*unstructured.Unstructured) (*DomainReconciler, *fake.Clientset, func(context.Context)) {
	t.Helper()
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{v1alpha1.DomainGVR: "DDSDomainList"}
	runtimeObjs := make([]runtime.Object, len(objs))
	for i, o := range objs {
		runtimeObjs[i] = o
	}
	dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, runtimeObjs...)
	factory := dynamicinformer.NewDynamicSharedInformerFactory(dynClient, 0)
	informer := factory.ForResource(v1alpha1.DomainGVR).Informer()

	k8sClient := fake.NewClientset()
	r := &DomainReconciler{Client: k8sClient, Informer: informer}

	start := func(ctx context.Context) {
		factory.Start(ctx.Done())
		factory.WaitForCacheSync(ctx.Done())
		go func() { _ = r.Run(ctx, 1) }()
	}
	return r, k8sClient, start
}

func TestDomainReconciler_CreatesNetworkPolicy(t *testing.T) {
	obj := domainObj("prod", "main", 3, true)
	_, k8sClient, start := newTestReconciler(t, obj)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start(ctx)

	name := netpolicy.NamePrefix + "main"
	if !waitFor(func() bool {
		_, err := k8sClient.NetworkingV1().NetworkPolicies("prod").Get(ctx, name, metav1.GetOptions{})
		return err == nil
	}) {
		t.Fatalf("NetworkPolicy %s/%s was never created", "prod", name)
	}

	np, err := k8sClient.NetworkingV1().NetworkPolicies("prod").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(np.Spec.PolicyTypes) != 2 {
		t.Errorf("PolicyTypes = %v", np.Spec.PolicyTypes)
	}
}

func TestDomainReconciler_DeletesOnRemoval(t *testing.T) {
	obj := domainObj("prod", "main", 3, true)
	r, k8sClient, start := newTestReconciler(t, obj)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start(ctx)

	name := netpolicy.NamePrefix + "main"
	if !waitFor(func() bool {
		_, err := k8sClient.NetworkingV1().NetworkPolicies("prod").Get(ctx, name, metav1.GetOptions{})
		return err == nil
	}) {
		t.Fatal("NetworkPolicy was never created")
	}

	// Simulate the DDSDomain being deleted from the informer's store
	// directly (avoids depending on the dynamic fake client's watch
	// delivery timing) and re-run reconcile for that key.
	if err := r.Informer.GetStore().Delete(obj); err != nil {
		t.Fatalf("store delete: %v", err)
	}
	if err := r.reconcile(ctx, "prod/main"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	_, err := k8sClient.NetworkingV1().NetworkPolicies("prod").Get(ctx, name, metav1.GetOptions{})
	if !errors.IsNotFound(err) {
		t.Fatalf("expected NetworkPolicy to be deleted, got err=%v", err)
	}
}

func TestDomainReconciler_NotIsolated_NoPolicy(t *testing.T) {
	obj := domainObj("staging", "main", 1, false)
	_, k8sClient, start := newTestReconciler(t, obj)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start(ctx)

	// Give reconciliation a moment; then assert nothing was created.
	time.Sleep(200 * time.Millisecond)
	list, err := k8sClient.NetworkingV1().NetworkPolicies("staging").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("expected no NetworkPolicy, got %d", len(list.Items))
	}
}

func TestDomainReconciler_UpdatesExistingPolicy(t *testing.T) {
	obj := domainObj("prod", "main", 1, true)
	r, k8sClient, start := newTestReconciler(t, obj)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start(ctx)

	name := netpolicy.NamePrefix + "main"
	if !waitFor(func() bool {
		_, err := k8sClient.NetworkingV1().NetworkPolicies("prod").Get(ctx, name, metav1.GetOptions{})
		return err == nil
	}) {
		t.Fatal("initial NetworkPolicy was never created")
	}

	// Mutate the object in the store to a different domain ID and
	// reconcile directly — this exercises the update (not just create)
	// path in applyPolicy.
	updated := domainObj("prod", "main", 5, true)
	if err := r.Informer.GetStore().Update(updated); err != nil {
		t.Fatalf("store update: %v", err)
	}
	if err := r.reconcile(ctx, "prod/main"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	np, err := k8sClient.NetworkingV1().NetworkPolicies("prod").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantStart, _ := netpolicy.DomainPortRange(5)
	if np.Spec.Ingress[0].Ports[0].Port.IntValue() != wantStart {
		t.Fatalf("policy not updated to new domain's port range: got %d, want %d",
			np.Spec.Ingress[0].Ports[0].Port.IntValue(), wantStart)
	}
}

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
