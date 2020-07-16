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
	"time"

	"github.com/go-logr/logr"
	capm "github.com/gpsingh-1991/cluster-api-provider-metamorph/api/v1alpha3"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	requeueAfter = time.Second * 30
)

// MetamorphClusterReconciler reconciles a MetamorphCluster object
type MetamorphClusterReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Log      logr.Logger
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=metamorphclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=metamorphclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

// Reconcile reads that state of the cluster for a MetamorphCluster object and makes changes based on the state read
// and what is in the MetamorphCluster.Spec
func (r *MetamorphClusterReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.TODO()
	log := r.Log.WithValues("namespace", req.Namespace, "metamorphcluster", req.Name)

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

	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef on MetamorphCluster")
		return ctrl.Result{}, nil
	}

	if util.IsPaused(cluster, metamorphCluster) {
		log.Info("MetamorphCluster or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{Requeue: true, RequeueAfter: requeueAfter}, nil
	}

	log = log.WithValues("cluster", cluster.Name)

	patchHelper, err := patch.NewHelper(metamorphCluster, r)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to initialize patch helper")
	}

	// Always patch the metamorphCluster when exiting this function so we can persist any MetamorphCluster changes.
	defer func() {
		if err := patchHelper.Patch(ctx, metamorphCluster); err != nil {
			if reterr == nil {
				log.Error(err, "failed to Patch metamorphCluster")
				reterr = errors.Wrapf(err, "error patching MetamorphCluster %s/%s", metamorphCluster.Namespace, metamorphCluster.Name)
			}
		}
	}()

	// Handle deleted clusters
	if !metamorphCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, log, patchHelper, cluster, metamorphCluster)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, log, patchHelper, cluster, metamorphCluster)
}

func (r *MetamorphClusterReconciler) reconcileDelete(ctx context.Context, log logr.Logger, patchHelper *patch.Helper, cluster *clusterv1.Cluster, metamorphCluster *capm.MetamorphCluster) (ctrl.Result, error) {
	log.Info("Reconciling Cluster delete")

	//clusterName := fmt.Sprintf("%s-%s", cluster.Namespace, cluster.Name)

	log.Info("Metamorph cluster deleted successfully")

	// Cluster is deleted so remove the finalizer.
	if err := patchHelper.Patch(ctx, metamorphCluster); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *MetamorphClusterReconciler) reconcileNormal(ctx context.Context, log logr.Logger, patchHelper *patch.Helper, cluster *clusterv1.Cluster, metamorphCluster *capm.MetamorphCluster) (ctrl.Result, error) {
	log.Info("Reconciling Cluster")

	// If the MetamorphCluster doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(metamorphCluster, capm.ClusterFinalizer)
	// Register the finalizer immediately to avoid orphaning Metamorph resources on delete
	if err := patchHelper.Patch(ctx, metamorphCluster); err != nil {
		return reconcile.Result{}, err
	}

	log.Info("Got Cluster info==", metamorphCluster.Spec.ControlPlaneEndpoint.Host)

	metamorphCluster.Status.Ready = true
	log.Info("Reconciled Cluster created successfully")
	return ctrl.Result{}, nil
}

func (r *MetamorphClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capm.MetamorphCluster{}).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: util.ClusterToInfrastructureMapFunc(
					capm.GroupVersion.WithKind("MetamorphCluster"),
				),
			},
		).
		Complete(r)
}
