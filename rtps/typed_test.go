// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Tests for Milestone 17 ("ROS 2 / rmw Compatibility")'s general-purpose
// additions to rtps: typed publisher/subscriber factories (carrying a real
// DDS type name instead of the "CDR_BLOB" sentinel) and the endpoint/GUID
// discovery introspection surface the ros2 package builds on.

package rtps_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/rtps"
)

// domain 150 is dedicated to this file's tests, distinct from every other
// domain constant already claimed elsewhere in this package's test suite.
const typedTestDomain = dds.Domain(150)

func TestTypedEndpoints_LocalDeliveryAndTypeName(t *testing.T) {
	p, err := rtps.New(typedTestDomain)
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	const wantType = "std_msgs::msg::dds_::String_"

	sub, err := dds.NewSubscriberWithType(p, "test/typed/basic", wantType, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriberWithType: %v", err)
	}
	defer sub.Close()

	pub, err := dds.NewPublisherWithType(p, "test/typed/basic", wantType, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisherWithType: %v", err)
	}
	defer pub.Close()

	want := []byte("hello ros2")
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: same-participant sample not delivered")
	}

	dp, ok := p.(rtps.EndpointDiscoveryProvider)
	if !ok {
		t.Fatal("participant does not implement EndpointDiscoveryProvider")
	}
	var sawWriter, sawReader bool
	for _, ep := range dp.DiscoveredEndpoints() {
		if ep.Topic != "test/typed/basic" {
			continue
		}
		if ep.TypeName != wantType {
			t.Errorf("endpoint TypeName = %q, want %q", ep.TypeName, wantType)
		}
		if !ep.Local {
			t.Errorf("endpoint Local = false, want true (own endpoint)")
		}
		if ep.IsWriter {
			sawWriter = true
		} else {
			sawReader = true
		}
	}
	if !sawWriter || !sawReader {
		t.Errorf("DiscoveredEndpoints: sawWriter=%v sawReader=%v, want both true", sawWriter, sawReader)
	}
}

func TestNewPublisher_DefaultsToCDRBlobTypeName(t *testing.T) {
	p, err := rtps.New(dds.Domain(151))
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("test/typed/default", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	dp, ok := p.(rtps.EndpointDiscoveryProvider)
	if !ok {
		t.Fatal("participant does not implement EndpointDiscoveryProvider")
	}
	found := false
	for _, ep := range dp.DiscoveredEndpoints() {
		if ep.Topic == "test/typed/default" && ep.IsWriter {
			found = true
			if ep.TypeName != "CDR_BLOB" {
				t.Errorf("TypeName = %q, want CDR_BLOB (default)", ep.TypeName)
			}
		}
	}
	if !found {
		t.Fatal("did not find the endpoint just created")
	}
}

func TestTypedEndpointFactory_UnsupportedBackend(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	if _, err := dds.NewPublisherWithType(p, "test/typed/mock", "any::Type_", dds.DefaultQoS); !errors.Is(err, dds.ErrTypeNameUnsupported) {
		t.Errorf("NewPublisherWithType(mock): err = %v, want ErrTypeNameUnsupported", err)
	}
	if _, err := dds.NewSubscriberWithType(p, "test/typed/mock", "any::Type_", dds.DefaultQoS); !errors.Is(err, dds.ErrTypeNameUnsupported) {
		t.Errorf("NewSubscriberWithType(mock): err = %v, want ErrTypeNameUnsupported", err)
	}
}

func TestGUIDProvider(t *testing.T) {
	p, err := rtps.New(dds.Domain(152))
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	gp, ok := p.(rtps.GUIDProvider)
	if !ok {
		t.Fatal("participant does not implement GUIDProvider")
	}
	var zero rtps.GUID
	if gp.GUID() == zero {
		t.Error("participant GUID is zero")
	}

	pub, err := p.NewPublisher("test/typed/guid", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	wgp, ok := pub.(rtps.GUIDProvider)
	if !ok {
		t.Fatal("publisher does not implement GUIDProvider")
	}
	if wgp.GUID() == zero {
		t.Error("publisher GUID is zero")
	}
	if wgp.GUID().Prefix != gp.GUID().Prefix {
		t.Error("publisher GUID prefix does not match its participant's")
	}
}
