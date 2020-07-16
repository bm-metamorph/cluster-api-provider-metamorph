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

	resty "github.com/go-resty/resty/v2"
	"github.com/go-logr/logr"
	capm "github.com/gpsingh-1991/cluster-api-provider-metamorph/api/v1alpha3"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	infrastructurev1alpha3 "github.com/metamorph/cluster-api-provider-metamorph/api/v1alpha3"
)

const (
	waitForClusterInfrastructureReadyDuration = 15 * time.Second
	machineControllerName                     = "MetamorphMachine-controller"
	RetryIntervalInstanceStatus               = 10 * time.Second
	metamorphEndpoint												=	"http://metamorph-api.metamorph:8080/"
)

// MetamorphMachineReconciler reconciles a MetamorphMachine object
type MetamorphMachineReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=metamorphmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=metamorphmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *MetamorphMachineReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = context.TODO()
	logger = r.Log.WithValues("namespace", req.Namespace, "MeamorphMachine", req.Name)

	// Fetch the Metamorphmachine instance
	metamorphMachine := &capm.MetamorphMachine{}
	err := r.Get(ctx, req.NamespacedName, metamorphMachine)
	if err != nil{
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the Machine
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		logger.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil
	}

	if isPaused(cluster, metamorphMachine) {
		logger.Info("MetamorphMachine or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("cluster", cluster.Name)

	metamorphCluster := &capm.MetamorphCluster{}

	metamorphClusterName := client.ObjectKey{
		Namespace: metamorphMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}

	if err := r.Client.Get(ctx, metamorphClusterName, metamorphCluster); err != nil {
		logger.Info("MetamorphCluster is not available yet")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("metamorphCluster", metamorphCluster.Name)

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(metamorphMachine, r)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always patch the metamorphMachine when exiting this function so we can persist any metamorphMachine changes.
	defer func() {
		if err := patchHelper.Patch(ctx, metamorphMachine); err != nil {
			if reterr == nil {
				reterr = err
			}
		}
	}()

	// Handle deleted machines
	if !metamorphMachine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, logger, patchHelper, machine, metamorphMachine, cluster, metamorphCluster)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, logger, patchHelper, machine, metamorphMachine, cluster, metamorphCluster)
}

// SetupWithManager sets
func (r *MetamorphMachineReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	controller, err := NewControllerManagedBy(mgr).
		For(&capm.MetamorphMachine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("MetamorphMachine")),
			},
		).
		Watches(
			&source.Kind{Type: &infrav1.MetmorphCluster{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.MetamorphClusterToMetamorphMachines)},
		).
		WithEventFilter(pausePredicates).
		Build(r)

		if err != nil {
			return err
		}

		return controller.Watch(
			&source.Kind{Type: &clusterv1.Cluster{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: handler.ToRequestsFunc(r.requeueMetamorphMachinesForUnpausedCluster),
			},
			predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldCluster := e.ObjectOld.(*clusterv1.Cluster)
					newCluster := e.ObjectNew.(*clusterv1.Cluster)
					return oldCluster.Spec.Paused && !newCluster.Spec.Paused
				},
				CreateFunc: func(e event.CreateEvent) bool {
					cluster := e.Object.(*clusterv1.Cluster)
					return !cluster.Spec.Paused
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false
				},
			},
		)
	}

func (r *MetamorphMachineReconciler) reconcileNormal(ctx context.Context, logger logr.Logger, patchHelper *patch.Helper, machine *clusterv1.Machine, metamorphMachine *capm.MetamorphMachine, cluster *clusterv1.Cluster, metamorphCluster *capm.MetamorphCluster) (_ ctrl.Result, reterr error) {
	// If the MetamorphMachine is in an error state, return early.
	if metamorphMachine.Status.FailureReason != nil || metamorphMachine.Status.FailureMessage != nil {
		logger.Info("Error state detected, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Add finalizer to MetamorphMachine
	controllerutil.AddFinalizer(metamorphMachine, capm.MachineFinalizer)
	// Register the finalizer immediately to avoid orphaning Metamorph resources on delete
	if err := patchHelper.Patch(ctx, metamorphMachine); err != nil {
		return ctrl.Result{}, err
	}

	if !cluster.Status.InfrastructureReady {
		logger.Info("Cluster infrastructure is not ready yet, requeuing machine")
		return ctrl.Result{RequeueAfter: waitForClusterInfrastructureReadyDuration}, nil
	}

	if machine.Spec.Bootstrap.DataSecretName == nil {
		logger.Info("Waiting for bootstrap data to be available")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	userData, err := r.getBootstrapData(machine, metamorphMachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Creating Machine")

	clusterName := fmt.Sprintf("%s-%s", cluster.ObjectMeta.Namespace, cluster.Name)

	if !metamorphMachine.Spec.IPMIDetails.Address {
		logger.Info("Node Address missing.")
		return ctrl.Result{}, nil
	}

	nodeIPAddress := strings.Split(metamorphMachine.Spec.IPMIDetails.Address, "/")[2]
	log.Info("Node IPMI IP address retrieved ", nodeIPAddress)

	secret := &corev1.Secret{}
	key := types.NamespacedName{
		Namespace: machine.Namespace,
		Name: metamorphMachine.Spec.IPMIDetails.CredentialsName,
	}
	if err := r.Client.Get(context.TODO(), key, secret); err != nil {
		return "", errors.Wrapf(err, "failed to retrieve IPMI Credentials for Metamorph Machine %s/%s", machine.Namespace, metamorphMachine.Name)
	}
	userName, ok := secret.Data["username"]
	password, ok := secret.Data["password"]

	crdinfoString := " { \"name\" : \"%v\"," +
		" \"ISOURL\" : \"%v\", " +
		" \"ISOChecksum\" : \"%v\", " +
		" \"IPMIUser\": \"%v\", " +
		" \"IPMIPassword\" : \"%v\", " +
		" \"IPMIIP\" : \"%v\", " +
		" \"disableCertVerification\" : %v }"

	crdinfoString = fmt.Sprintf(crdinfoString,
		metamorphMachine.Name,
		metamorphMachine.Spec.Image.URL,
		metamorphMachine.Spec.Image.Checksum,
		userName,
		password,
		nodeIPAddress,
		metamorphMachine.Spec.IPMIDetails.DisableCertificateVerification,
	)
	log.Info(crdinfoString)
	metamorphNodeEndpoint := metamorphEndpoint + "node"

	resultBody := make(map[string]interface{})
	restyClient := resty.New()
	restyClient.SetDebug(true)

	resp, err := restyClient.R().EnableTrace().
		SetHeader("Content-Type", "application/json").
		SetBody([]byte(crdinfoString)).Post(metamorphNodeEndpoint)

	if (err == nil) && (resp.StatusCode() == http.StatusOK) {
		err = json.Unmarshal(resp.Body(), &resultBody)
	} else {
		log.Info("Trace info:", resp.Request.TraceInfo())
		return nil, errors.Wrap(err, fmt.Sprintf("Post request failed : URL - %v, reqbody - %v", metamorphNodeEndpoint, crdinfoString ))
	}

	nodeUUID := resultBody["result"]
	log.Info(fmt.Sprintf("Node UUID retrieved %v\n", nodeUUID))

	metamorphMachine.Status.ProviderID = nodeUUID

}

func (r *MetamorphMachineReconciler) getBootstrapData(machine *clusterv1.Machine, metamorphMachine *capm.MetamorphMachine) (string, error) {
	if machine.Spec.Bootstrap.DataSecretName == nil {
		return "", errors.New("error retrieving bootstrap data: linked Machine's bootstrap.dataSecretName is nil")
	}

	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: machine.Namespace, Name: *machine.Spec.Bootstrap.DataSecretName}
	if err := r.Client.Get(context.TODO(), key, secret); err != nil {
		return "", errors.Wrapf(err, "failed to retrieve bootstrap data secret for Metamorph Machine %s/%s", machine.Namespace, metamorphMachine.Name)
	}

	value, ok := secret.Data["value"]
	if !ok {
		return "", errors.New("error retrieving bootstrap data: secret value key is missing")
	}

	return base64.StdEncoding.EncodeToString(value), nil
}

func (r *MetamorphMachineReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, patchHelper *patch.Helper, machine *clusterv1.Machine, metamorphMachine *capm.MetamorphMachine, cluster *clusterv1.Cluster, metamorphCluster *capm.MetamorphCluster) (_ ctrl.Result, reterr error) {
		logger.Info("Handling deleted MetamorphMachine")

		clusterName := fmt.Sprintf("%s-%s", cluster.ObjectMeta.Namespace, cluster.Name)
