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

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	capm "github.com/bm-metamorph/cluster-api-provider-metamorph/api/v1alpha3"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"

	infrastructurev1alpha3 "github.com/metamorph/cluster-api-provider-metamorph/api/v1alpha3"
)

// MetamorphClusterReconciler reconciles a MetamorphCluster object
type MetamorphClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=metamorphclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=metamorphclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

// Reconcile reads that state of the cluster for a MetamorphCluster object and makes changes based on the state read
// and what is in the MetamorphCluster.Spec
func (r *MetamorphClusterReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.Background()
	clusterLog := r.Log.WithValues("namespace", req.Namespace, "metamorphcluster", req.Name)

	// Fetch the MetamorphCluster instance
	metamorphCluster := &capm.MetamorphCluster{}
	err := r.Get(ctx, req.NamespacedName, metamorphCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, metamorphCluster.ObjectMeta)
	if err != nil {
		error := capierrors.InvalidConfigurationClusterError
		metamorphCluster.Status.FailureReason = &error
		metamorphCluster.Status.FailureMessage = pointer.StringPtr("Unable to get owner cluster")
		return ctrl.Result{}, err

	}

	if isPaused(cluster, metamorphCluster) {
		clusterLog.Info("MetamorphCluster or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{Requeue: true, RequeueAfter: requeueAfter}, nil
	}

	if cluster == nil {
		clusterLog.Info("Cluster Controller has not yet set OwnerRef on MetamorphCluster")
		return ctrl.Result{}, nil
	}

	clusterLog = clusterLog.WithValues("cluster", cluster.Name)

	patchHelper, err := patch.NewHelper(metamorphCluster, r)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to initialize patch helper")
	}

	// Always patch the openStackCluster when exiting this function so we can persist any OpenStackCluster changes.
	defer func() {
		if err := patchHelper.Patch(ctx, metamorphCluster); err != nil {
			if reterr == nil {
				clusterLog.Error(err, "failed to Patch metamorphCluster")
				reterr = errors.Wrapf(err, "error patching MetamorphCluster %s/%s", metamorphCluster.Namespace, metamorphCluster.Name)
			}
		}
	}()

	// Handle deleted clusters
	if !metamorphCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, clusterLog, patchHelper, cluster, metamorphCluster)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, clusterLog, patchHelper, cluster, metamorphCluster)
}

func (r *MetamorphClusterReconciler) reconcileDelete(ctx context.Context, log logr.Logger, patchHelper *patch.Helper, cluster *clusterv1.Cluster, metamorphCluster *capm.MetamorphCluster) (ctrl.Result, error) {
	log.Info("Reconciling Cluster delete")

	clusterName := fmt.Sprintf("%s-%s", cluster.Namespace, cluster.Name)
	osProviderClient, clientOpts, err := provider.NewClientFromCluster(r.Client, openStackCluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	networkingService, err := networking.NewService(osProviderClient, clientOpts, log)
	if err != nil {
		return reconcile.Result{}, err
	}

	loadBalancerService, err := loadbalancer.NewService(osProviderClient, clientOpts, log, openStackCluster.Spec.UseOctavia)
	if err != nil {
		return reconcile.Result{}, err
	}

	if openStackCluster.Spec.ManagedAPIServerLoadBalancer {
		err = loadBalancerService.DeleteLoadBalancer(clusterName, openStackCluster)
		if err != nil {
			return reconcile.Result{}, errors.Errorf("failed to delete load balancer: %v", err)
		}
	}

	// Delete other things
	if openStackCluster.Status.WorkerSecurityGroup != nil {
		log.Info("Deleting worker security group", "name", openStackCluster.Status.WorkerSecurityGroup.Name)
		err := networkingService.DeleteSecurityGroups(openStackCluster.Status.WorkerSecurityGroup)
		if err != nil {
			return reconcile.Result{}, errors.Errorf("failed to delete security group: %v", err)
		}
	}

	if openStackCluster.Status.ControlPlaneSecurityGroup != nil {
		log.Info("Deleting control plane security group", "name", openStackCluster.Status.ControlPlaneSecurityGroup.Name)
		err := networkingService.DeleteSecurityGroups(openStackCluster.Status.ControlPlaneSecurityGroup)
		if err != nil {
			return reconcile.Result{}, errors.Errorf("failed to delete security group: %v", err)
		}
	}

	if openStackCluster.Status.Network.Router != nil {
		log.Info("Deleting router", "name", openStackCluster.Status.Network.Router.Name)
		if err := networkingService.DeleteRouter(openStackCluster.Status.Network); err != nil {
			return ctrl.Result{}, errors.Errorf("failed to delete router: %v", err)
		}
		log.Info("OpenStack router deleted successfully")
	}

	// if NodeCIDR was not set, no network was created.
	if openStackCluster.Status.Network != nil && openStackCluster.Spec.NodeCIDR != "" {
		log.Info("Deleting network", "name", openStackCluster.Status.Network.Name)
		if err := networkingService.DeleteNetwork(openStackCluster.Status.Network); err != nil {
			return ctrl.Result{}, errors.Errorf("failed to delete network: %v", err)
		}
		log.Info("OpenStack network deleted successfully")
	}

	log.Info("OpenStack cluster deleted successfully")

	// Cluster is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(openStackCluster, infrav1.ClusterFinalizer)
	log.Info("Reconciled Cluster delete successfully")
	if err := patchHelper.Patch(ctx, openStackCluster); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *MetamorphClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha3.MetamorphCluster{}).
		Complete(r)
}
