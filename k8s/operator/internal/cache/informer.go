package cache

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8scache "k8s.io/client-go/tools/cache"

	"github.com/SoundMatt/go-DDS/k8s/operator/api/v1alpha1"
)

// Participants caches DDSParticipant specs keyed by Key(namespace, name).
type Participants struct {
	store *Store[*v1alpha1.DDSParticipantSpec]
}

// NewParticipants returns an empty Participants cache.
func NewParticipants() *Participants {
	return &Participants{store: NewStore[*v1alpha1.DDSParticipantSpec]()}
}

// Get implements internal/webhook's ParticipantLookup.
func (p *Participants) Get(namespace, name string) (*v1alpha1.DDSParticipantSpec, bool) {
	return p.store.Get(Key(namespace, name))
}

// Len reports the number of cached participants (readiness/debug logging).
func (p *Participants) Len() int { return p.store.Len() }

// EventHandler returns a ResourceEventHandler that keeps this cache in
// sync with a DDSParticipant SharedIndexInformer's add/update/delete
// events. Malformed objects (spec doesn't decode) are dropped rather than
// cached stale or zero-valued — a webhook miss (fail-open, see
// internal/webhook) is safer than injecting garbage config.
func (p *Participants) EventHandler() k8scache.ResourceEventHandler {
	return k8scache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { p.upsert(obj) },
		UpdateFunc: func(_, newObj interface{}) { p.upsert(newObj) },
		DeleteFunc: func(obj interface{}) { p.remove(obj) },
	}
}

func (p *Participants) upsert(obj interface{}) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	spec, err := v1alpha1.ParticipantSpecFromUnstructured(u)
	if err != nil {
		return
	}
	p.store.Set(Key(u.GetNamespace(), u.GetName()), spec)
}

func (p *Participants) remove(obj interface{}) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	p.store.Delete(Key(u.GetNamespace(), u.GetName()))
}

// Domains caches DDSDomain specs keyed by Key(namespace, name). DDSDomain
// is namespace-scoped and governs the namespace it lives in — see
// ROADMAP.md Milestone 15 and api/v1alpha1's package doc.
type Domains struct {
	store *Store[*v1alpha1.DDSDomainSpec]
}

// NewDomains returns an empty Domains cache.
func NewDomains() *Domains {
	return &Domains{store: NewStore[*v1alpha1.DDSDomainSpec]()}
}

// Get implements internal/webhook's DomainLookup.
func (d *Domains) Get(namespace, name string) (*v1alpha1.DDSDomainSpec, bool) {
	return d.store.Get(Key(namespace, name))
}

// Len reports the number of cached domains (readiness/debug logging).
func (d *Domains) Len() int { return d.store.Len() }

// EventHandler returns a ResourceEventHandler keeping this cache in sync
// with a DDSDomain SharedIndexInformer.
func (d *Domains) EventHandler() k8scache.ResourceEventHandler {
	return k8scache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { d.upsert(obj) },
		UpdateFunc: func(_, newObj interface{}) { d.upsert(newObj) },
		DeleteFunc: func(obj interface{}) { d.remove(obj) },
	}
}

func (d *Domains) upsert(obj interface{}) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	spec, err := v1alpha1.DomainSpecFromUnstructured(u)
	if err != nil {
		return
	}
	d.store.Set(Key(u.GetNamespace(), u.GetName()), spec)
}

func (d *Domains) remove(obj interface{}) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	d.store.Delete(Key(u.GetNamespace(), u.GetName()))
}

// NamespaceDomains caches the v1alpha1.DomainLabel value of every
// Namespace, keyed by namespace name, resolving "which DDSDomain object
// governs this namespace" for the webhook without a live API read.
type NamespaceDomains struct{ store *Store[string] }

// NewNamespaceDomains returns an empty NamespaceDomains cache.
func NewNamespaceDomains() *NamespaceDomains {
	return &NamespaceDomains{store: NewStore[string]()}
}

// Get returns the DDSDomain name bound to namespace, if any.
func (n *NamespaceDomains) Get(namespace string) (string, bool) {
	return n.store.Get(namespace)
}

// Len reports the number of namespaces with a bound domain.
func (n *NamespaceDomains) Len() int { return n.store.Len() }

// EventHandler returns a ResourceEventHandler keeping this cache in sync
// with a core/v1 Namespace SharedIndexInformer (delivered as unstructured
// by the same dynamic informer factory used for the CRDs — see
// internal/controller).
func (n *NamespaceDomains) EventHandler() k8scache.ResourceEventHandler {
	return k8scache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { n.upsert(obj) },
		UpdateFunc: func(_, newObj interface{}) { n.upsert(newObj) },
		DeleteFunc: func(obj interface{}) { n.remove(obj) },
	}
}

func (n *NamespaceDomains) upsert(obj interface{}) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	if domain, found := u.GetLabels()[v1alpha1.DomainLabel]; found && domain != "" {
		n.store.Set(u.GetName(), domain)
		return
	}
	n.store.Delete(u.GetName())
}

func (n *NamespaceDomains) remove(obj interface{}) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	n.store.Delete(u.GetName())
}

// toUnstructured unwraps the interface{} delivered by a SharedIndexInformer
// event, handling the DeletedFinalStateUnknown wrapper client-go uses when
// a delete is observed after a watch gap (see k8scache.DeletedFinalStateUnknown).
func toUnstructured(obj interface{}) (*unstructured.Unstructured, bool) {
	if tomb, ok := obj.(k8scache.DeletedFinalStateUnknown); ok {
		obj = tomb.Obj
	}
	u, ok := obj.(*unstructured.Unstructured)
	return u, ok
}
