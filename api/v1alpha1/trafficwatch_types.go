/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TrafficWatchSpec defines the desired state of TrafficWatch
type TrafficWatchSpec struct {
	// MaxBandwidthPercent defines amount of traffic that will cause a Node
	// to be matrked as "unfit" as a floating point value
	MaxBandwidthPercent string `json:"maxBandwidthPercent"`
}

// TrafficWatchStatus defines the observed state of TrafficWatch
type TrafficWatchStatus struct {
	// Nodes current status of nodes
	Nodes map[string]CurrentNodeTraffic `json:"nodes"`
}

type CurrentNodeTraffic struct {
	// CurrentBandwidthPercent shows average use of bandwidth in last 10s
	CurrentBandwidthPercent string `json:"currentBandwidthPercent"`

	// CurrentTransmitTotal
	CurrentTransmitTotal int64 `json:"currentTransmitTotal"`

	// Time
	Time int64 `json:"time"`

	// Unfit is marked as "unfit"
	Unfit bool `json:"unfit"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// TrafficWatch is the Schema for the trafficwatches API
type TrafficWatch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrafficWatchSpec   `json:"spec,omitempty"`
	Status TrafficWatchStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TrafficWatchList contains a list of TrafficWatch
type TrafficWatchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TrafficWatch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TrafficWatch{}, &TrafficWatchList{})
}
