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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	capm "github.com/bm-metamorph/cluster-api-provider-metamorph/api/v1alpha3"
	"github.com/go-logr/logr"
	resty "github.com/go-resty/resty/v2"
	"gopkg.in/yaml.v2"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	//"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	waitForClusterInfrastructureReadyDuration = 15 * time.Second
	machineControllerName                     = "MetamorphMachine-controller"
)

var pausePredicates = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		return !util.IsPaused(nil, e.MetaNew)
	},
	CreateFunc: func(e event.CreateEvent) bool {
		return !util.IsPaused(nil, e.Meta)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return !util.IsPaused(nil, e.Meta)
	},
}
var metamorphEndpoint string

// MetamorphMachineReconciler reconciles a MetamorphMachine object
type MetamorphMachineReconciler struct {
	client.Client
	Log      logr.Logger
	Recorder record.EventRecorder
}

func init() {
	metamorphEndpoint = os.Getenv("METAMORPH_ENDPOINT")
	if metamorphEndpoint == "" {
		fmt.Fprintf(os.Stderr, "Cannot start: No METAMORPH_ENDPOINT variable set\n")
		os.Exit(1)
	}
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=metamorphmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=metamorphmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile reconiles
func (r *MetamorphMachineReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.TODO()
	logger := r.Log.WithValues("namespace", req.Namespace, "MeamtorphMachine", req.Name)

	// Fetch the Metamorphmachine instance
	metamorphMachine := &capm.MetamorphMachine{}
	err := r.Get(ctx, req.NamespacedName, metamorphMachine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the Machine.
	obj := metamorphMachine.ObjectMeta
	for _, ref := range obj.OwnerReferences {
		logger.Info("machine ref: ", ref.Kind)
	}
	machine, err := util.GetOwnerMachine(ctx, r.Client, metamorphMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		logger.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("machine", machine.Name)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		logger.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil
	}

	if util.IsPaused(cluster, metamorphMachine) {
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
func (r *MetamorphMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capm.MetamorphMachine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: util.MachineToInfrastructureMapFunc(capm.GroupVersion.WithKind("MetamorphMachine")),
			},
		).
		Watches(
			&source.Kind{Type: &capm.MetamorphCluster{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: handler.ToRequestsFunc(r.MetamorphClusterToMetamorphMachines),
			},
		).
		Complete(r)

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

	bootstrapData, err := r.getBootstrapData(machine, metamorphMachine)
	if err != nil {
		logger.Info(bootstrapData)
		return ctrl.Result{}, err
	}
	logger.Info("bootstrapData")

	logger.Info(bootstrapData)

	userData, err := r.getUserData(machine, metamorphMachine)
	if err != nil {
		return ctrl.Result{}, err
	}
	userDataDecoded, err := base64.StdEncoding.DecodeString(userData)
	if err != nil {
		fmt.Println("decode error:", err)
	}

	logger.Info("Creating Machine")

	var metamorphNode capm.Node

	logger.Info("*******")
	yaml.Unmarshal(userDataDecoded, &metamorphNode)

	if metamorphMachine.Spec.IPMIDetails.Address == "" {
		logger.Info("Node Address missing.")
		return ctrl.Result{}, nil
	}
	nodeIPAddress := strings.Split(metamorphMachine.Spec.IPMIDetails.Address, "/")[2]
	logger.Info("Node IPMI IP address retrieved ", nodeIPAddress)

	secret := &corev1.Secret{}
	key := types.NamespacedName{
		Namespace: machine.Namespace,
		Name:      metamorphMachine.Spec.IPMIDetails.CredentialsName,
	}
	if err := r.Client.Get(context.TODO(), key, secret); err != nil {
		err = errors.Wrapf(err, "failed to retrieve IPMI Credentials for Metamorph Machine %s/%s", machine.Namespace, metamorphMachine.Name)
		return ctrl.Result{}, err
	}
	IPMIUserName, ok := secret.Data["username"]
	IPMIPassword, ok := secret.Data["password"]

	if !ok {
		err := errors.New("error retrieving bootstrap data: secret value key is missing")
		return ctrl.Result{}, err
	}

	//logger.Info(metamorphNode.IPMIIP)

	//IpmiIP := strings.Split(metamorphMachine.Spec.IPMIDetails.Address, "/")[2]

	metamorphNode.Name = metamorphMachine.Name
	metamorphNode.IPMIIP = nodeIPAddress
	metamorphNode.IPMIUser = string(IPMIUserName)
	metamorphNode.IPMIPassword = string(IPMIPassword)
	metamorphNode.ISOURL = metamorphMachine.Spec.Image.URL
	metamorphNode.ISOChecksum = metamorphMachine.Spec.Image.Checksum
	metamorphNode.CloudInit = bootstrapData

	//logger.Info(metamorphNode.IPMIIP)
	//logger.Info("*******")

	// out, err := yaml.Marshal(&metamorphNode)
	// fmt.Println(string(out))
	// fmt.Println(err)

	//clusterName := fmt.Sprintf("%s-%s", cluster.ObjectMeta.Namespace, cluster.Name)

	return r.getOrCreate(ctx, logger, patchHelper, machine, metamorphMachine, &metamorphNode, cluster, metamorphCluster)

	//return ctrl.Result{}, nil
	//return ctrl.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
}

func (r *MetamorphMachineReconciler) getOrCreate(ctx context.Context, logger logr.Logger, patchHelper *patch.Helper, machine *clusterv1.Machine, metamorphMachine *capm.MetamorphMachine, metamorphNode *capm.Node, cluster *clusterv1.Cluster, metamorphCluster *capm.MetamorphCluster) (_ ctrl.Result, reterr error) {

	logger.Info(" From Inside getOrCreate \n\n")
	//nodeID := "0ed63609-7cdc-4074-81a9-b7b0e9a8a5cd"
	//metamorphMachine.Status.ProviderID = &nodeID

	if metamorphMachine.Status.ProviderID != nil {

		metamorphNodeEndpoint := fmt.Sprintf("%s/node/%s", metamorphEndpoint, *metamorphMachine.Status.ProviderID)

		client := resty.New()
		resp, err := client.R().EnableTrace().Get(metamorphNodeEndpoint)
		json.Unmarshal(resp.Body(), metamorphNode)

		//fmt.Println(resp)
		fmt.Println(metamorphNode.State)
		fmt.Println(err)
		metamorphMachine.Status.InstanceState = metamorphNode.State
		if metamorphNode.State == "readywait" {
			metamorphNode.State = "ready"
			reqBody, err := json.Marshal(&metamorphNode)
			resp, err := client.R().EnableTrace().SetHeader("Content-Type", "application/json").SetBody([]byte(reqBody)).Put(metamorphNodeEndpoint)
			fmt.Println(resp)
			fmt.Println(err)
		}
		if metamorphNode.State == "setupreadywait" {
			metamorphNode.State = "setupready"
			reqBody, err := json.Marshal(&metamorphNode)
			resp, err := client.R().EnableTrace().SetHeader("Content-Type", "application/json").SetBody([]byte(reqBody)).Put(metamorphNodeEndpoint)
			fmt.Println(resp)
			fmt.Println(err)
		}
		if metamorphNode.State == "deployed" {
			//metamorphMachine.Status.ProviderID =
			providerID := fmt.Sprintf("metamorph://%s", *metamorphMachine.Status.ProviderID)
			metamorphMachine.Spec.ProviderID = &providerID
			metamorphMachine.Status.Ready = true
			return ctrl.Result{}, nil

		}
		if metamorphNode.State == "failed" {
			//metamorphMachine.Status.ProviderID =
			providerID := fmt.Sprintf("metamorph://%s", *metamorphMachine.Status.ProviderID)
			metamorphMachine.Spec.ProviderID = &providerID
			metamorphMachine.Status.Ready = false
			return ctrl.Result{}, nil

		}

		if metamorphMachine.Annotations == nil {
			metamorphMachine.Annotations = map[string]string{}
		}
		metamorphMachine.Annotations["cluster-api-provider-metamorph"] = "true"

	} else {
		reqBody, err := json.Marshal(&metamorphNode)
		//fmt.Println(string(out))
		fmt.Println(err)

		metamorphNodeEndpoint := fmt.Sprintf("%s/node", metamorphEndpoint)
		logger.Info(metamorphNodeEndpoint)

		resultBody := make(map[string]interface{})
		restyClient := resty.New()
		restyClient.SetDebug(true)

		fmt.Println("Node Doesn't exist. Creating a new Node")
		resp, err := restyClient.R().EnableTrace().
			SetHeader("Content-Type", "application/json").
			SetBody([]byte(reqBody)).Post(metamorphNodeEndpoint)

		if (err == nil) && (resp.StatusCode() == http.StatusOK) {
			err = json.Unmarshal(resp.Body(), &resultBody)
		} else {
			logger.Info("Trace info:", resp.Request.TraceInfo())
			err = errors.Wrap(err, fmt.Sprintf("Post request failed : URL - %v, reqbody - %v", metamorphNodeEndpoint, reqBody))
			return ctrl.Result{}, err
		}

		nodeUUID := resultBody["result"].(string)
		logger.Info(fmt.Sprintf("Node UUID retrieved %v\n", nodeUUID))

		metamorphMachine.Status.ProviderID = &nodeUUID
		//return ctrl.Result{}, nil
	}

	fmt.Println("Requeue after 30 sec")
	return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
}

// MetamorphClusterToMetamorphMachines func
func (r *MetamorphMachineReconciler) MetamorphClusterToMetamorphMachines(o handler.MapObject) []ctrl.Request {
	result := []ctrl.Request{}
	c, ok := o.Object.(*capm.MetamorphCluster)
	if !ok {
		r.Log.Error(errors.Errorf("expected a MetamorphCluster but got a %T", o.Object), "failed to get MetamorphMachine for MetamorphCluster")
		return nil
	}
	log := r.Log.WithValues("MetamorphCluster", c.Name, "Namespace", c.Namespace)

	cluster, err := util.GetOwnerCluster(context.TODO(), r.Client, c.ObjectMeta)
	switch {
	case apierrors.IsNotFound(err) || cluster == nil:
		return result
	case err != nil:
		log.Error(err, "failed to get owning cluster")
		return result
	}

	labels := map[string]string{clusterv1.ClusterLabelName: cluster.Name}
	machineList := &clusterv1.MachineList{}
	if err := r.List(context.TODO(), machineList, client.InNamespace(c.Namespace), client.MatchingLabels(labels)); err != nil {
		log.Error(err, "failed to list Machines")
		return nil
	}
	for _, m := range machineList.Items {
		if m.Spec.InfrastructureRef.Name == "" {
			continue
		}
		name := client.ObjectKey{Namespace: m.Namespace, Name: m.Spec.InfrastructureRef.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}

func (r *MetamorphMachineReconciler) requeueMetamorphMachinesForUnpausedCluster(o handler.MapObject) []ctrl.Request {
	c, ok := o.Object.(*clusterv1.Cluster)
	if !ok {
		r.Log.Error(errors.Errorf("expected a Cluster but got a %T", o.Object), "failed to get MetamorphMachines for unpaused Cluster")
		return nil
	}

	// Don't handle deleted clusters
	if !c.ObjectMeta.DeletionTimestamp.IsZero() {
		return nil
	}

	return r.requestsForCluster(c.Namespace, c.Name)
}

func (r *MetamorphMachineReconciler) requestsForCluster(namespace, name string) []ctrl.Request {
	log := r.Log.WithValues("Cluster", name, "Namespace", namespace)
	labels := map[string]string{clusterv1.ClusterLabelName: name}
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(context.TODO(), machineList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		log.Error(err, "failed to get owned Machines")
		return nil
	}

	result := make([]ctrl.Request, 0, len(machineList.Items))
	for _, m := range machineList.Items {
		if m.Spec.InfrastructureRef.Name != "" {
			result = append(result, ctrl.Request{NamespacedName: client.ObjectKey{Namespace: m.Namespace, Name: m.Spec.InfrastructureRef.Name}})
		}
	}
	return result
}

func (r *MetamorphMachineReconciler) getUserData(machine *clusterv1.Machine, metamorphMachine *capm.MetamorphMachine) (string, error) {
	if metamorphMachine.Spec.UserData.Name == "" {
		return "", errors.New("error retrieving Userdata: linked metamorphMachine's userData.Name is nil")
	}

	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: metamorphMachine.Spec.UserData.Namespace, Name: metamorphMachine.Spec.UserData.Name}
	if err := r.Client.Get(context.TODO(), key, secret); err != nil {
		return "", errors.Wrapf(err, "failed to retrieve Userdata secret for MetaMorph Machine %s/%s", machine.Namespace, metamorphMachine.Name)
	}

	value, ok := secret.Data["userData"]
	if !ok {
		return "", errors.New("error retrieving userdata: secret value key is missing")
	}

	return base64.StdEncoding.EncodeToString(value), nil
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

	//clusterName := fmt.Sprintf("%s-%s", cluster.ObjectMeta.Namespace, cluster.Name)
	return ctrl.Result{}, nil
}
