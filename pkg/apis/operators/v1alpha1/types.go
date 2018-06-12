package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CouchbaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Couchbase `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Couchbase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              CouchbaseSpec   `json:"spec"`
	Status            CouchbaseStatus `json:"status,omitempty"`
}

type CouchbaseSpec struct {
	Size int32 `json:"size"`
	Image string `json:"image"`
}

type CouchbaseStatus struct {
	Nodes []string `json:"nodes"`
}
