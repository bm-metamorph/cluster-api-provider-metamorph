/*


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

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// ClusterFinalizer allows ReconcileMetamorphCluster to clean up Metamorph resources associated with MetamorphCluster before
	// removing it from the apiserver.
	ClusterFinalizer = "metamorphcluster.infrastructure.cluster.x-k8s.io"
)

// MetamorphClusterSpec defines the desired state of MetamorphCluster
type MetamorphClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	//ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint"`
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint"`
}

// MetamorphClusterStatus defines the observed state of MetamorphCluster
type MetamorphClusterStatus struct {
	Ready bool `json:"ready"`

	// FailureReason indicates that there is a fatal problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	// +optional
	FailureReason *capierrors.ClusterStatusError `json:"failureReason,omitempty"`

	// FailureMessage indicates that there is a fatal problem reconciling the
	// state, and will be set to a descriptive error message.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=metamorphclusters,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster to which this MetamorphCluster belongs"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Cluster infrastructure is ready for Metamorph instances"
// +kubebuilder:printcolumn:name="Network",type="string",JSONPath=".status.network.id",description="Network the cluster is using"
// +kubebuilder:printcolumn:name="Subnet",type="string",JSONPath=".status.network.subnet.id",description="Subnet the cluster is using"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".status.network.apiServerLoadBalancer.ip",description="API Endpoint",priority=1

// MetamorphCluster is the Schema for the metamorphclusters API
type MetamorphCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MetamorphClusterSpec   `json:"spec,omitempty"`
	Status MetamorphClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MetamorphClusterList contains a list of MetamorphCluster
type MetamorphClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MetamorphCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MetamorphCluster{}, &MetamorphClusterList{})
}
