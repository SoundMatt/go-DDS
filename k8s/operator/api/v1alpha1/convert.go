package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ParticipantGVR is the GroupVersionResource the dynamic client and
// informer factory watch DDSParticipant custom resources under.
var ParticipantGVR = schema.GroupVersionResource{Group: Group, Version: Version, Resource: ParticipantResource}

// DomainGVR is the GroupVersionResource the dynamic client and informer
// factory watch DDSDomain custom resources under.
var DomainGVR = schema.GroupVersionResource{Group: Group, Version: Version, Resource: DomainResource}

// ParticipantSpecFromUnstructured extracts a DDSParticipantSpec from a
// DDSParticipant custom resource's unstructured representation (as
// delivered by the dynamic client/informer).
func ParticipantSpecFromUnstructured(u *unstructured.Unstructured) (*DDSParticipantSpec, error) {
	spec, found, err := unstructured.NestedMap(u.Object, "spec")
	if err != nil {
		return nil, fmt.Errorf("v1alpha1: reading spec: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("v1alpha1: %s/%s has no spec", u.GetNamespace(), u.GetName())
	}
	out := &DDSParticipantSpec{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(spec, out); err != nil {
		return nil, fmt.Errorf("v1alpha1: decoding DDSParticipantSpec: %w", err)
	}
	return out, nil
}

// DomainSpecFromUnstructured extracts a DDSDomainSpec from a DDSDomain
// custom resource's unstructured representation.
func DomainSpecFromUnstructured(u *unstructured.Unstructured) (*DDSDomainSpec, error) {
	spec, found, err := unstructured.NestedMap(u.Object, "spec")
	if err != nil {
		return nil, fmt.Errorf("v1alpha1: reading spec: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("v1alpha1: %s/%s has no spec", u.GetNamespace(), u.GetName())
	}
	out := &DDSDomainSpec{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(spec, out); err != nil {
		return nil, fmt.Errorf("v1alpha1: decoding DDSDomainSpec: %w", err)
	}
	return out, nil
}
