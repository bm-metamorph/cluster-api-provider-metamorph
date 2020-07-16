/*
Copyright 2019 The Kubernetes Authors.

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

package manager

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	capm "github.com/bm-metamorph/cluster-api-provider-metamorph/api/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capi "sigs.k8s.io/cluster-api/api/v1alpha3"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MetamorphClusterManager is an interface for ClusterManager
type MetamorphClusterManager interface {
	Create(context.Context) error
	Delete() error
	UpdateClusterStatus() error
	SetFinalizer()
	UnsetFinalizer()
	CountDescendants(context.Context) (int, error)
}

// ClusterManager is responsible for performing machine reconciliation
type ClusterManager struct {
	client client.Client

	Cluster          *capi.Cluster
	MetamorphCluster *capm.MetamorphCluster
	Log              logr.Logger
}

// NewClusterManager returns a new helper for managing a cluster with a given name.
func NewClusterManager(client client.Client, cluster *capi.Cluster,
	metamorphCluster *capm.MetamorphCluster,
	clusterLog logr.Logger) (MetamorphClusterManager, error) {

	if metamorphCluster == nil {
		return nil, errors.New("MetamorphCluster is required when creating a ClusterManager")
	}
	if cluster == nil {
		return nil, errors.New("Cluster is required when creating a ClusterManager")
	}

	return &ClusterManager{
		client:           client,
		MetamorphCluster: metamorphCluster,
		Cluster:          cluster,
		Log:              clusterLog,
	}, nil
}

// SetFinalizer sets finalizer
func (s *ClusterManager) SetFinalizer() {
	// If the MetamorphCluster doesn't have finalizer, add it.
	if !Contains(s.MetamorphCluster.ObjectMeta.Finalizers, capm.ClusterFinalizer) {
		s.MetamorphCluster.ObjectMeta.Finalizers = append(
			s.MetamorphCluster.ObjectMeta.Finalizers, capm.ClusterFinalizer,
		)
	}
}

// UnsetFinalizer unsets finalizer
func (s *ClusterManager) UnsetFinalizer() {
	// Cluster is deleted so remove the finalizer.
	s.MetamorphCluster.ObjectMeta.Finalizers = Filter(
		s.MetamorphCluster.ObjectMeta.Finalizers, capm.ClusterFinalizer,
	)
}

// Create creates a cluster manager for the cluster.
func (s *ClusterManager) Create(ctx context.Context) error {

	config := s.MetamorphCluster.Spec
	if err != nil {
		// Should have been picked earlier. Do not requeue
		s.setError("Invalid MetamorphCluster provided", capierrors.InvalidConfigurationClusterError)
		return err
	}

	// clear an error if one was previously set
	s.clearError()

	return nil
}

// ControlPlaneEndpoint returns cluster controlplane endpoint
func (s *ClusterManager) ControlPlaneEndpoint() ([]capm.APIEndpoint, error) {
	//Get IP address from spec, which gets it from posted cr yaml
	endPoint := s.MetamorphCluster.Spec.ControlPlaneEndpoint
	var err error

	if endPoint.Host == "" || endPoint.Port == 0 {
		s.Log.Error(err, "Host IP or PORT not set")
		return nil, err
	}

	return []capm.APIEndpoint{
		{
			Host: endPoint.Host,
			Port: endPoint.Port,
		},
	}, nil
}

// Delete function, no-op for now
func (s *ClusterManager) Delete() error {
	return nil
}

// UpdateClusterStatus updates a machine object's status.
func (s *ClusterManager) UpdateClusterStatus() error {

	// Get APIEndpoints from  MetamorphCluster Spec
	_, err := s.ControlPlaneEndpoint()

	if err != nil {
		s.MetamoprhCluster.Status.Ready = false
		s.setError("Invalid ControlPlaneEndpoint values", capierrors.InvalidConfigurationClusterError)
		return err
	}

	// Mark the metamorphCluster ready
	s.MetamorphCluster.Status.Ready = true
	now := metav1.Now()
	s.MetamorphCluster.Status.LastUpdated = &now
	return nil
}

// setError sets the FailureMessage and FailureReason fields on the machine and logs
// the message. It assumes the reason is invalid configuration, since that is
// currently the only relevant MachineStatusError choice.
func (s *ClusterManager) setError(message string, reason capierrors.ClusterStatusError) {
	s.MetamoprhCluster.Status.FailureMessage = &message
	s.MetamorphCluster.Status.FailureReason = &reason
}

// clearError removes the ErrorMessage from the machine's Status if set. Returns
// nil if ErrorMessage was already nil. Returns a RequeueAfterError if the
// machine was updated.
func (s *ClusterManager) clearError() {
	if s.MetamorphCluster.Status.FailureMessage != nil || s.MetamorphCluster.Status.FailureReason != nil {
		s.MetamorphCluster.Status.FailureMessage = nil
		s.MetamorphCluster.Status.FailureReason = nil
	}
}

// CountDescendants will return the number of descendants objects of the
// metamorphCluster
func (s *ClusterManager) CountDescendants(ctx context.Context) (int, error) {
	// Verify that no metamorphmachine depend on the metamorphcluster
	descendants, err := s.listDescendants(ctx)
	if err != nil {
		s.Log.Error(err, "Failed to list descendants")

		return 0, err
	}

	nbDescendants := len(descendants.Items)

	if nbDescendants > 0 {
		s.Log.Info(
			"metamorphCluster still has descendants - need to requeue", "descendants",
			nbDescendants,
		)
	}
	return nbDescendants, nil
}

// listDescendants returns a list of all Machines, for the cluster owning the
// metamorphCluster.
func (s *ClusterManager) listDescendants(ctx context.Context) (capi.MachineList, error) {

	machines := capi.MachineList{}
	cluster, err := util.GetOwnerCluster(ctx, s.client,
		s.MetamorphCluster.ObjectMeta,
	)
	if err != nil {
		return machines, err
	}

	listOptions := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{
			capi.ClusterLabelName: cluster.Name,
		}),
	}

	if s.client.List(ctx, &machines, listOptions...) != nil {
		errMsg := fmt.Sprintf("failed to list metamorphmachines for cluster %s/%s", cluster.Namespace, cluster.Name)
		return machines, errors.Wrapf(err, errMsg)
	}

	return machines, nil
}
