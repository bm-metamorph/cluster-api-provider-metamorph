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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// MachineFinalizer allows ReconcileMetamorphMachine to clean up resources associated with MetamorphMachine before
	// removing it from the apiserver.
	MachineFinalizer = "metamorphmachine.infrastructure.cluster.x-k8s.io"
)

// MetamorphMachineSpec defines the desired state of MetamorphMachine
type MetamorphMachineSpec struct {
	Online bool `json:"online"`

	// Image is the image to be deployed.
	Image Image `json:"image"`

	UserData *corev1.SecretReference `json:"userData,omitempty"`

	IPMIDetails IPMIDetails `json:"IPMISecret,omitempty"`
}

// MetamorphMachineStatus defines the observed state of MetamorphMachine
type MetamorphMachineStatus struct {

	// ProviderID is the unique identifier as specified by the provider.
	ProviderID *string `json:"providerID,omitempty"`
	// Ready is true when the provider resource is ready.
	// +optional
	Ready bool `json:"ready"`

	// ddresses is a list of addresses assigned to the machine.
	// +optional
	Addresses []corev1.NodeAddress `json:"addresses,omitempty"`

	FailureReason *capierrors.MachineStatusError `json:"failureReason,omitempty"`

	// FailureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	// +optional
	FailureMessage *string `json:"errorMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=metamorphmachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster to which this MetamorphMachine belongs"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.instanceState",description="Metamorph node state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine ready status"
// +kubebuilder:printcolumn:name="InstanceID",type="string",JSONPath=".spec.providerID",description="Metamorph instance ID"
// +kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Machine\")].name",description="Machine object which owns with this MetamorphMachine"

// MetamorphMachine is the Schema for the metamorphmachines API
type MetamorphMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MetamorphMachineSpec   `json:"spec,omitempty"`
	Status MetamorphMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MetamorphMachineList contains a list of MetamorphMachine
type MetamorphMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MetamorphMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MetamorphMachine{}, &MetamorphMachineList{})
}
