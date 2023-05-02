//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Code generated by controller-gen. DO NOT EDIT.

package common

import (
	"k8s.io/api/core/v1"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KeyValueOrSecretKeySelectorPair) DeepCopyInto(out *KeyValueOrSecretKeySelectorPair) {
	*out = *in
	in.Value.DeepCopyInto(&out.Value)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KeyValueOrSecretKeySelectorPair.
func (in *KeyValueOrSecretKeySelectorPair) DeepCopy() *KeyValueOrSecretKeySelectorPair {
	if in == nil {
		return nil
	}
	out := new(KeyValueOrSecretKeySelectorPair)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KeyValuePair) DeepCopyInto(out *KeyValuePair) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KeyValuePair.
func (in *KeyValuePair) DeepCopy() *KeyValuePair {
	if in == nil {
		return nil
	}
	out := new(KeyValuePair)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LocalObjectRef) DeepCopyInto(out *LocalObjectRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LocalObjectRef.
func (in *LocalObjectRef) DeepCopy() *LocalObjectRef {
	if in == nil {
		return nil
	}
	out := new(LocalObjectRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NameValuePair) DeepCopyInto(out *NameValuePair) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NameValuePair.
func (in *NameValuePair) DeepCopy() *NameValuePair {
	if in == nil {
		return nil
	}
	out := new(NameValuePair)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PartialObjectRef) DeepCopyInto(out *PartialObjectRef) {
	*out = *in
	out.TypeMeta = in.TypeMeta
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PartialObjectRef.
func (in *PartialObjectRef) DeepCopy() *PartialObjectRef {
	if in == nil {
		return nil
	}
	out := new(PartialObjectRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ValueOrSecretKeySelector) DeepCopyInto(out *ValueOrSecretKeySelector) {
	*out = *in
	if in.From != nil {
		in, out := &in.From, &out.From
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
	if in.Value != nil {
		in, out := &in.Value, &out.Value
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ValueOrSecretKeySelector.
func (in *ValueOrSecretKeySelector) DeepCopy() *ValueOrSecretKeySelector {
	if in == nil {
		return nil
	}
	out := new(ValueOrSecretKeySelector)
	in.DeepCopyInto(out)
	return out
}