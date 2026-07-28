// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package ros2

import "testing"

func TestValidNodeName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"talker", true},
		{"_talker", true},
		{"talker_1", true},
		{"Talker2", true},
		{"", false},
		{"1talker", false},     // must not start with a digit
		{"talker-1", false},    // hyphen not allowed
		{"talker/ns", false},   // no slashes in a bare node name
		{"talker name", false}, // no spaces
		{"talker.name", false}, // no dots
	}
	for _, c := range cases {
		if got := ValidNodeName(c.name); got != c.want {
			t.Errorf("ValidNodeName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestValidNamespace(t *testing.T) {
	cases := []struct {
		ns   string
		want bool
	}{
		{"", true},
		{"/", true},
		{"/robot1", true},
		{"/robot1/sensors", true},
		{"robot1", false},   // must start with "/"
		{"/robot1/", false}, // must not end with "/" (except root)
		{"/robot-1", false}, // hyphen not allowed in a segment
		{"//robot1", false}, // empty segment
	}
	for _, c := range cases {
		if got := ValidNamespace(c.ns); got != c.want {
			t.Errorf("ValidNamespace(%q) = %v, want %v", c.ns, got, c.want)
		}
	}
}

func TestFullyQualifiedName(t *testing.T) {
	cases := []struct {
		ns, name, want string
	}{
		{"/", "camera", "/camera"},
		{"", "camera", "/camera"},
		{"/robot1", "camera", "/robot1/camera"},
		{"/robot1", "/absolute/topic", "/absolute/topic"}, // absolute wins
		{"/", "/absolute/topic", "/absolute/topic"},
	}
	for _, c := range cases {
		if got := FullyQualifiedName(c.ns, c.name); got != c.want {
			t.Errorf("FullyQualifiedName(%q, %q) = %q, want %q", c.ns, c.name, got, c.want)
		}
	}
}

func TestTopicNameMangling(t *testing.T) {
	cases := []struct {
		ros string
		dds string
	}{
		{"/chatter", "rt/chatter"},
		{"/robot1/camera/image", "rt/robot1/camera/image"},
		{"/", "rt/"},
	}
	for _, c := range cases {
		if got := ToDDSTopicName(c.ros); got != c.dds {
			t.Errorf("ToDDSTopicName(%q) = %q, want %q", c.ros, got, c.dds)
		}
		got, ok := FromDDSTopicName(c.dds)
		if !ok {
			t.Errorf("FromDDSTopicName(%q): ok = false, want true", c.dds)
		}
		if got != c.ros {
			t.Errorf("FromDDSTopicName(%q) = %q, want %q", c.dds, got, c.ros)
		}
	}
}

func TestFromDDSTopicName_NonROS2(t *testing.T) {
	cases := []string{
		DiscoveryTopicName, // "ros_discovery_info" — never "rt"-mangled
		"plain/go-dds/topic",
		"rq/some/serviceRequest", // service wire name, not a plain topic
		"",
	}
	for _, ddsTopic := range cases {
		if _, ok := FromDDSTopicName(ddsTopic); ok {
			t.Errorf("FromDDSTopicName(%q): ok = true, want false", ddsTopic)
		}
	}
}

func TestTypeSupportName(t *testing.T) {
	got := TypeSupportName("std_msgs", "msg", "String")
	want := "std_msgs::msg::dds_::String_"
	if got != want {
		t.Fatalf("TypeSupportName = %q, want %q", got, want)
	}
	pkg, subfolder, msgType, ok := ParseTypeSupportName(got)
	if !ok {
		t.Fatal("ParseTypeSupportName: ok = false")
	}
	if pkg != "std_msgs" || subfolder != "msg" || msgType != "String" {
		t.Errorf("ParseTypeSupportName = (%q, %q, %q), want (std_msgs, msg, String)", pkg, subfolder, msgType)
	}
}

func TestParseTypeSupportName_NotROS2(t *testing.T) {
	if _, _, _, ok := ParseTypeSupportName("CDR_BLOB"); ok {
		t.Error("ParseTypeSupportName(CDR_BLOB): ok = true, want false")
	}
}
