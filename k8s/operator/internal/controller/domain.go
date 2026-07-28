// Package controller reconciles DDSDomain custom resources into
// NetworkPolicy objects (ROADMAP.md, Milestone 15, "Kubernetes Operator" —
// "DDSDomain CRD: domain-per-namespace isolation with network policy
// generation") and keeps internal/cache warm for the admission webhook.
package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	k8scache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/SoundMatt/go-DDS/k8s/operator/api/v1alpha1"
	"github.com/SoundMatt/go-DDS/k8s/operator/internal/netpolicy"
)

// DomainReconciler watches DDSDomain custom resources (via the informer
// passed to Run) and applies the corresponding NetworkPolicy — created,
// updated in place, or deleted when IsolateNamespace turns false —
// idempotently keyed by the deterministic name netpolicy.Build assigns.
type DomainReconciler struct {
	Client   kubernetes.Interface
	Informer k8scache.SharedIndexInformer
	Log      *slog.Logger

	queue workqueue.TypedRateLimitingInterface[string]
}

func (r *DomainReconciler) logger() *slog.Logger {
	if r.Log != nil {
		return r.Log
	}
	return slog.Default()
}

// Run wires the reconciler's event handlers onto Informer and processes
// the work queue with the given number of workers until ctx is done. It
// blocks until the informer's cache has synced before starting workers,
// and returns once ctx is canceled.
func (r *DomainReconciler) Run(ctx context.Context, workers int) error {
	r.queue = workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]())
	defer r.queue.ShutDown()

	handle, err := r.Informer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    r.enqueue,
		UpdateFunc: func(_, newObj interface{}) { r.enqueue(newObj) },
		DeleteFunc: r.enqueue,
	})
	if err != nil {
		return fmt.Errorf("controller: registering DDSDomain event handler: %w", err)
	}
	defer func() { _ = r.Informer.RemoveEventHandler(handle) }()

	if !k8scache.WaitForCacheSync(ctx.Done(), r.Informer.HasSynced) {
		return errors.New("controller: DDSDomain informer cache never synced")
	}

	for i := 0; i < workers; i++ {
		go r.worker(ctx)
	}
	<-ctx.Done()
	return nil
}

func (r *DomainReconciler) enqueue(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		tomb, ok := obj.(k8scache.DeletedFinalStateUnknown)
		if !ok {
			return
		}
		u, ok = tomb.Obj.(*unstructured.Unstructured)
		if !ok {
			return
		}
	}
	r.queue.Add(u.GetNamespace() + "/" + u.GetName())
}

func (r *DomainReconciler) worker(ctx context.Context) {
	for {
		key, shutdown := r.queue.Get()
		if shutdown {
			return
		}
		if err := r.reconcile(ctx, key); err != nil {
			r.logger().Error("controller: reconcile failed, requeueing", "key", key, "error", err)
			r.queue.AddRateLimited(key)
		} else {
			r.queue.Forget(key)
		}
		r.queue.Done(key)
	}
}

// reconcile fetches the current DDSDomain (if it still exists), renders
// the desired NetworkPolicy, and reconciles the cluster's actual
// NetworkPolicy to match — including deleting a previously generated
// policy when the DDSDomain is gone or IsolateNamespace is now false.
func (r *DomainReconciler) reconcile(ctx context.Context, key string) error {
	ns, name, err := k8scache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("controller: invalid key %q: %w", key, err)
	}

	var desired *networkingv1.NetworkPolicy
	obj, exists, err := r.Informer.GetStore().GetByKey(key)
	if err != nil {
		return fmt.Errorf("controller: fetching %s from store: %w", key, err)
	}
	if exists {
		u, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return fmt.Errorf("controller: unexpected object type for %s: %T", key, obj)
		}
		spec, err := v1alpha1.DomainSpecFromUnstructured(u)
		if err != nil {
			return fmt.Errorf("controller: decoding DDSDomain %s: %w", key, err)
		}
		desired = netpolicy.Build(ns, name, spec)
	}

	policyName := netpolicy.NamePrefix + name
	if desired == nil {
		return r.deletePolicy(ctx, ns, policyName)
	}
	return r.applyPolicy(ctx, desired)
}

func (r *DomainReconciler) deletePolicy(ctx context.Context, namespace, name string) error {
	err := r.Client.NetworkingV1().NetworkPolicies(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("controller: deleting NetworkPolicy %s/%s: %w", namespace, name, err)
	}
	return nil
}

// applyPolicy creates the NetworkPolicy if absent, or updates it in place
// (preserving resourceVersion) if the spec differs from what's live.
func (r *DomainReconciler) applyPolicy(ctx context.Context, desired *networkingv1.NetworkPolicy) error {
	client := r.Client.NetworkingV1().NetworkPolicies(desired.Namespace)

	existing, err := client.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, createErr := client.Create(ctx, desired, metav1.CreateOptions{}); createErr != nil {
			return fmt.Errorf("controller: creating NetworkPolicy %s/%s: %w", desired.Namespace, desired.Name, createErr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("controller: getting NetworkPolicy %s/%s: %w", desired.Namespace, desired.Name, err)
	}

	updated := existing.DeepCopy()
	updated.Labels = desired.Labels
	updated.Spec = desired.Spec
	if _, err := client.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("controller: updating NetworkPolicy %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return nil
}
