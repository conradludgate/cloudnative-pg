/*
Copyright The CloudNativePG Contributors

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

// Package controllers contains the controller of the CRD
package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/log"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/resources/instance"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

const (
	dbPodOwnerKey = ".metadata.controller.database"
	// pvcOwnerKey                   = ".metadata.controller.database"
	dbJobOwnerKey = ".metadata.controller.database"
	// poolerClusterKey              = ".spec.cluster.name"
	// disableDefaultQueriesSpecPath = ".spec.monitoring.disableDefaultQueries"
)

// var apiGVString = apiv1.GroupVersion.String()

// DatabaseReconciler reconciles a Database objects
type DatabaseReconciler struct {
	client.Client

	DiscoveryClient discovery.DiscoveryInterface
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder

	*instance.StatusClient
}

// NewDatabaseReconciler creates a new DatabaseReconciler initializing it
func NewDatabaseReconciler(mgr manager.Manager, discoveryClient *discovery.DiscoveryClient) *DatabaseReconciler {
	return &DatabaseReconciler{
		StatusClient:    instance.NewStatusClient(),
		DiscoveryClient: discoveryClient,
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		Recorder:        mgr.GetEventRecorderFor("cloudnative-pg-database"),
	}
}

// ErrNextLoop see utils.ErrNextLoop
// var ErrNextLoop = utils.ErrNextLoop

// Alphabetical order to not repeat or miss permissions
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=databases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=databases/status,verbs=get;watch;update;patch
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters,verbs=get
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;delete;patch;create;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;delete;patch;create;watch
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=get;list;delete;patch;create;watch
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get
// +kubebuilder:rbac:groups="",resources=secrets,verbs=create;list;get;watch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=create;patch;update;list;watch;get

// Reconcile is the operator reconcile loop
func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	contextLogger, ctx := log.SetupLogger(ctx)

	contextLogger.Debug(fmt.Sprintf("reconciling object %#q", req.NamespacedName))

	defer func() {
		contextLogger.Debug(fmt.Sprintf("object %#q has been reconciled", req.NamespacedName))
	}()

	var database apiv1.Database
	if err := r.Get(ctx, req.NamespacedName, &database); err != nil {
		if apierrs.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	switch database.Status.Phase {
	case apiv1.DatabasePhaseFailed, apiv1.DatabasePhaseCompleted:
		return ctrl.Result{}, nil
	case "":
		r.RegisterPhase(ctx, &database, apiv1.DatabasePhasePending, "just starting")
	}

	clusterNamespace := database.Spec.Cluster.Namespace
	clusterName := database.Spec.Cluster.Name

	r.Recorder.Eventf(&database, "Normal", "FindingCluster",
		"hello we are looking for cluster %v", clusterName)

	var cluster apiv1.Cluster
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: clusterNamespace,
		Name:      clusterName,
	}, &cluster); err != nil {
		if apierrs.IsNotFound(err) {
			r.Recorder.Eventf(&database, "Warning", "FindingCluster",
				"Unknown cluster %v, will retry in 30 seconds", clusterName)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		tryFlagDatabaseAsFailed(ctx, r.Client, &database, fmt.Errorf("while getting cluster %s: %w", clusterName, err))
		r.Recorder.Eventf(&database, "Warning", "FindingCluster",
			"Error getting cluster %v, will not retry: %s", clusterName, err.Error())
		return ctrl.Result{}, nil
	}

	r.Recorder.Eventf(&database, "Normal", "FindingCluster",
		"hello we have found cluster %v", clusterName)

	// Run the inner reconcile loop. Translate any ErrNextLoop to an errorless return
	result, err := r.reconcile(ctx, &cluster, &database)
	if errors.Is(err, ErrNextLoop) {
		return result, nil
	}
	return result, err
}

func tryFlagDatabaseAsFailed(
	ctx context.Context,
	cli client.Client,
	database *apiv1.Database,
	err error,
) {
	contextLogger := log.FromContext(ctx)
	origBackup := database.DeepCopy()
	database.Status.SetAsFailed(err)

	if err := cli.Status().Patch(ctx, database, client.MergeFrom(origBackup)); err != nil {
		contextLogger.Error(err, "while flagging backup as failed")
	}
}

// Inner reconcile loop. Anything inside can require the reconciliation loop to stop by returning ErrNextLoop
// nolint:gocognit
func (r *DatabaseReconciler) reconcile(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database) (ctrl.Result, error) {
	contextLogger := log.FromContext(ctx)

	if utils.IsReconciliationDisabled(&database.ObjectMeta) {
		contextLogger.Warning("Disable reconciliation loop annotation set, skipping the reconciliation.")
		return ctrl.Result{}, nil
	}

	// // IMPORTANT: the following call will delete conditions using
	// // invalid condition reasons.
	// //
	// // This operation is necessary to migrate from a version using
	// // the customized Condition structure to one using the standard
	// // one from K8S, that has more strict validations.
	// //
	// // The next reconciliation loop of the instance manager will
	// // recreate the dropped conditions.
	// err := r.removeConditionsWithInvalidReason(ctx, cluster)
	// if err != nil {
	// 	return ctrl.Result{}, err
	// }

	// // Make sure default values are populated.
	// err = r.setDefaults(ctx, cluster)
	// if err != nil {
	// 	return ctrl.Result{}, err
	// }

	// // Ensure we reconcile the orphan resources if present when we reconcile for the first time a cluster
	// if err := r.reconcileRestoredData(ctx, cluster); err != nil {
	// 	return ctrl.Result{}, fmt.Errorf("cannot reconcile restored Cluster: %w", err)
	// }

	// Ensure we have the required global objects
	if err := r.createPostgresDatabaseObjects(ctx, cluster, database); err != nil {
		return ctrl.Result{}, fmt.Errorf("cannot create Database auxiliary objects: %w", err)
	}

	// Update the status of this resource
	resources, err := r.getManagedDatabaseResources(ctx, cluster, database)
	if err != nil {
		contextLogger.Error(err, "Cannot extract the list of managed resources")
		return ctrl.Result{}, err
	}

	// Update the status section of this Cluster resource
	if err = r.updateResourceStatus(ctx, cluster, database, resources); err != nil {
		if apierrs.IsConflict(err) {
			// Requeue a new reconciliation cycle, as in this point we need
			// to quickly react the changes
			contextLogger.Debug("Conflict error while reconciling resource status", "error", err)
			return ctrl.Result{Requeue: true}, nil
		}

		return ctrl.Result{}, fmt.Errorf("cannot update the resource status: %w", err)
	}

	if cluster.Status.CurrentPrimary != "" &&
		cluster.Status.CurrentPrimary != cluster.Status.TargetPrimary {
		contextLogger.Info("There is a switchover or a failover "+
			"in progress, waiting for the operation to complete",
			"currentPrimary", cluster.Status.CurrentPrimary,
			"targetPrimary", cluster.Status.TargetPrimary)

		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	// // Get the replication status
	// instancesStatus := r.StatusClient.GetStatusFromInstances(ctx, resources.instances)

	// // we update all the cluster status fields that require the instances status
	// if err := r.updateClusterStatusThatRequiresInstancesState(ctx, cluster, instancesStatus); err != nil {
	// 	if apierrs.IsConflict(err) {
	// 		contextLogger.Debug("Conflict error while reconciling cluster status and instance state",
	// 			"error", err)
	// 		return ctrl.Result{Requeue: true}, nil
	// 	}
	// 	return ctrl.Result{}, fmt.Errorf("cannot update the instances status on the cluster: %w", err)
	// }

	// if err := instanceReconciler.ReconcileMetadata(ctx, r.Client, cluster, resources.instances); err != nil {
	// 	return ctrl.Result{}, err
	// }

	// if instancesStatus.AllReadyInstancesStatusUnreachable() {
	// 	contextLogger.Warning(
	// 		"Failed to extract instance status from ready instances. Attempting to requeue...",
	// 	)
	// 	registerPhaseErr := r.RegisterPhase(
	// 		ctx,
	// 		cluster,
	// 		"Instance Status Extraction Error: HTTP communication issue",
	// 		"Communication issue detected: The operator was unable to receive the status from all the ready instances. "+
	// 			"This may be due to network restrictions such as NetworkPolicy and/or any other network plugin setting. "+
	// 			"Please verify your network configuration.",
	// 	)
	// 	return ctrl.Result{RequeueAfter: 10 * time.Second}, registerPhaseErr
	// }

	// // Verify the architecture of all the instances and update the OnlineUpdateEnabled
	// // field in the status
	// onlineUpdateEnabled := configuration.Current.EnableInstanceManagerInplaceUpdates
	// fencedInstances, err := utils.GetFencedInstances(cluster.Annotations)
	// if err != nil {
	// 	contextLogger.Error(err, "while getting fenced instances")
	// 	return ctrl.Result{}, err
	// }

	// isArchitectureConsistent := r.checkPodsArchitecture(ctx, fencedInstances, &instancesStatus)
	// if !isArchitectureConsistent && onlineUpdateEnabled {
	// 	contextLogger.Info("Architecture mismatch detected, disabling instance manager online updates")
	// 	onlineUpdateEnabled = false
	// }

	// // The instance list is sorted and will present the primary as the first
	// // element, followed by the replicas, the most updated coming first.
	// // Pods that are not responding will be at the end of the list. We use
	// // the information reported by the instance manager to sort the
	// // instances. When we need to elect a new primary, we take the first item
	// // on this list.
	// //
	// // Here we check the readiness status of the first Pod as we can't
	// // promote an instance that is not ready from the Kubernetes
	// // point-of-view: the services will not forward traffic to it even if
	// // PostgreSQL is up and running.
	// //
	// // An instance can be up and running even if the readiness probe is
	// // negative: this is going to happen, i.e., when an instance is
	// // un-fenced, and the Kubelet still hasn't refreshed the status of the
	// // readiness probe.
	// if instancesStatus.Len() > 0 {
	// 	mostAdvancedInstance := instancesStatus.Items[0]
	// 	hasHTTPStatus := mostAdvancedInstance.HasHTTPStatus()
	// 	isPodReady := mostAdvancedInstance.IsPodReady

	// 	if hasHTTPStatus && !isPodReady {
	// 		// The readiness probe status from the Kubelet is not updated, so
	// 		// we need to wait for it to be refreshed
	// 		contextLogger.Info(
	// 			"Waiting for the Kubelet to refresh the readiness probe",
	// 			"mostAdvancedInstanceName", mostAdvancedInstance.Node,
	// 			"hasHTTPStatus", hasHTTPStatus,
	// 			"isPodReady", isPodReady)
	// 		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	// 	}
	// }

	// // If the user has requested to hibernate the cluster, we do that before
	// // ensuring the primary to be healthy. The hibernation starts from the
	// // primary Pod to ensure the replicas are in sync and doing it here avoids
	// // any unwanted switchover.
	// if result, err := hibernation.Reconcile(
	// 	ctx,
	// 	r.Client,
	// 	cluster,
	// 	resources.instances.Items,
	// ); result != nil || err != nil {
	// 	return *result, err
	// }

	// // We have already updated the status in updateResourceStatus call,
	// // so we need to issue an extra update when the OnlineUpdateEnabled changes.
	// // It's okay because it should not change often.
	// //
	// // We cannot merge this code with updateResourceStatus because
	// // it needs to run after retrieving the status from the pods,
	// // which is a time-expensive operation.
	// if err = r.updateOnlineUpdateEnabled(ctx, cluster, onlineUpdateEnabled); err != nil {
	// 	if apierrs.IsConflict(err) {
	// 		// Requeue a new reconciliation cycle, as in this point we need
	// 		// to quickly react the changes
	// 		contextLogger.Debug("Conflict error while reconciling online update", "error", err)
	// 		return ctrl.Result{Requeue: true}, nil
	// 	}

	// 	return ctrl.Result{}, fmt.Errorf("cannot update the resource status: %w", err)
	// }
	// result, err := r.handleSwitchover(ctx, cluster, resources, instancesStatus)
	// if err != nil {
	// 	return ctrl.Result{}, err
	// }
	// if result != nil {
	// 	return *result, nil
	// }

	// Updates all the objects managed by the controller
	return r.reconcileResources(ctx, cluster, database, resources)
}

// func (r *DatabaseReconciler) handleSwitchover(
// 	ctx context.Context,
// 	cluster *apiv1.Cluster,
// 	resources *managedResources,
// 	instancesStatus postgres.PostgresqlStatusList,
// ) (*ctrl.Result, error) {
// 	contextLogger := log.FromContext(ctx)
// 	if cluster.IsInstanceFenced(cluster.Status.CurrentPrimary) ||
// 		instancesStatus.ReportingMightBeUnavailable(cluster.Status.CurrentPrimary) {
// 		contextLogger.Info("The current primary instance is fenced or is still recovering from it," +
// 			" we won't trigger a switchover")
// 		return nil, nil
// 	}
// 	if cluster.Status.Phase == apiv1.PhaseInplaceDeletePrimaryRestart {
// 		if cluster.Status.ReadyInstances != cluster.Spec.Instances {
// 			contextLogger.Info("Waiting for the primary to be restarted without triggering a switchover")
// 			return nil, nil
// 		}
// 		contextLogger.Info("All instances ready, will proceed",
// 			"currentPrimary", cluster.Status.CurrentPrimary,
// 			"targetPrimary", cluster.Status.TargetPrimary)
// 		if err := r.RegisterPhase(ctx, cluster, apiv1.PhaseHealthy, ""); err != nil {
// 			return nil, err
// 		}
// 		return nil, nil
// 	}

// 	// Update the target primary name from the Pods status.
// 	// This means issuing a failover or switchover when needed.
// 	selectedPrimary, err := r.updateTargetPrimaryFromPods(ctx, cluster, instancesStatus, resources)
// 	if err != nil {
// 		if err == ErrWaitingOnFailOverDelay {
// 			contextLogger.Info("Waiting for the failover delay to expire")
// 			return &ctrl.Result{RequeueAfter: 1 * time.Second}, nil
// 		}
// 		if err == ErrWalReceiversRunning {
// 			contextLogger.Info("Waiting for all WAL receivers to be down to elect a new primary")
// 			return &ctrl.Result{RequeueAfter: 1 * time.Second}, nil
// 		}
// 		contextLogger.Info("Cannot update target primary: operation cannot be fulfilled. "+
// 			"An immediate retry will be scheduled",
// 			"cluster", cluster.Name)
// 		return &ctrl.Result{Requeue: true}, nil
// 	}
// 	if selectedPrimary != "" {
// 		// If we selected a new primary, stop the reconciliation loop here
// 		contextLogger.Info("Waiting for the new primary to notice the promotion request",
// 			"newPrimary", selectedPrimary)
// 		return &ctrl.Result{RequeueAfter: 1 * time.Second}, nil
// 	}

// 	// Primary is healthy, No switchover in progress.
// 	// If we have a currentPrimaryFailingSince timestamp, let's unset it.
// 	if cluster.Status.CurrentPrimaryFailingSinceTimestamp != "" {
// 		cluster.Status.CurrentPrimaryFailingSinceTimestamp = ""
// 		if err := r.Status().Update(ctx, cluster); err != nil {
// 			return nil, err
// 		}
// 	}

// 	return nil, nil
// }

func (r *DatabaseReconciler) getCluster(
	ctx context.Context,
	req ctrl.Request,
) (*apiv1.Cluster, error) {
	contextLogger := log.FromContext(ctx)
	cluster := &apiv1.Cluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		// This also happens when you delete a Cluster resource in k8s. If
		// that's the case, let's just wait for the Kubernetes garbage collector
		// to remove all the Pods of the cluster.
		if apierrs.IsNotFound(err) {
			contextLogger.Info("Resource has been deleted")
			return nil, nil
		}

		// This is a real error, maybe the RBAC configuration is wrong?
		return nil, fmt.Errorf("cannot get the managed resource: %w", err)
	}

	var namespace corev1.Namespace
	if err := r.Get(ctx, client.ObjectKey{Namespace: "", Name: req.Namespace}, &namespace); err != nil {
		// This is a real error, maybe the RBAC configuration is wrong?
		return nil, fmt.Errorf("cannot get the containing namespace: %w", err)
	}

	if !namespace.DeletionTimestamp.IsZero() {
		// This happens when you delete a namespace containing a Cluster resource. If that's the case,
		// let's just wait for the Kubernetes to remove all object in the namespace.
		return nil, nil
	}

	return cluster, nil
}

// func (r *DatabaseReconciler) setDefaults(ctx context.Context, cluster *apiv1.Cluster) error {
// 	contextLogger := log.FromContext(ctx)
// 	originCluster := cluster.DeepCopy()
// 	cluster.SetDefaults()
// 	if !reflect.DeepEqual(originCluster.Spec, cluster.Spec) {
// 		contextLogger.Info("Admission controllers (webhooks) appear to have been disabled. " +
// 			"Please enable them for this object/namespace")
// 		err := r.Patch(ctx, cluster, client.MergeFrom(originCluster))
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// reconcileResources updates all the objects managed by the controller
func (r *DatabaseReconciler) reconcileResources(
	ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database, resources *managedDatabaseResources,
) (ctrl.Result, error) {
	contextLogger := log.FromContext(ctx)

	r.Recorder.Event(database, "Normal", "ReconcileDatabase", "still reconciling")

	// Act on Pods and PVCs only if there is nothing that is currently being created or deleted
	if runningJobs := resources.countRunningJobs(); runningJobs > 0 {
		contextLogger.Debug("A job is currently running. Waiting", "count", runningJobs)
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	// // Delete Pods which have been evicted by the Kubelet
	// result, err := r.deleteEvictedOrUnscheduledInstances(ctx, cluster, resources)
	// if err != nil {
	// 	contextLogger.Error(err, "While deleting evicted pods")
	// 	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	// }
	// if result != nil {
	// 	return *result, err
	// }

	// TODO: move into a central waiting phase
	// If we are joining a node, we should wait for the process to finish
	if resources.countRunningJobs() > 0 {
		contextLogger.Debug("Waiting for jobs to finish",
			"clusterName", cluster.Name,
			"namespace", cluster.Namespace,
			"jobs", len(resources.jobs.Items))
		return ctrl.Result{RequeueAfter: 1 * time.Second}, ErrNextLoop
	}

	r.Recorder.Event(database, "Normal", "ReconcileDatabase", "we should start a job here probs")

	// if !resources.allInstancesAreActive() {
	// 	contextLogger.Debug("Instance pod not active. Retrying in one second.")

	// 	// Preserve phases that handle the in-place restart behaviour for the following reasons:
	// 	// 1. Technically: The Inplace phases help determine if a switchover is required.
	// 	// 2. Descriptive: They precisely describe the cluster's current state externally.
	// 	if cluster.IsInplaceRestartPhase() {
	// 		contextLogger.Debug("Cluster is in an in-place restart phase. Waiting...", "phase", cluster.Status.Phase)
	// 	} else {
	// 		// If not in an Inplace phase, notify that the reconciliation is halted due
	// 		// to an unready instance.
	// 		contextLogger.Debug("An instance is not ready. Pausing reconciliation...")

	// 		// Register a phase indicating some instances aren't active yet
	// 		if err := r.RegisterPhase(
	// 			ctx,
	// 			cluster,
	// 			apiv1.PhaseWaitingForInstancesToBeActive,
	// 			"Some instances are not yet active. Please wait.",
	// 		); err != nil {
	// 			return ctrl.Result{}, err
	// 		}
	// 	}

	// 	// Requeue reconciliation after a short delay
	// 	return ctrl.Result{RequeueAfter: time.Second}, nil
	// }

	// if res, err := persistentvolumeclaim.Reconcile(
	// 	ctx,
	// 	r.Client,
	// 	cluster,
	// 	resources.instances.Items,
	// 	resources.pvcs.Items,
	// ); !res.IsZero() || err != nil {
	// 	return res, err
	// }

	instances, err := GetManagedInstances(ctx, cluster, r.Client)
	if err != nil {
		r.Recorder.Event(database, "Warning", "ReconcileDatabase", "no instances?")
		contextLogger.Error(err, "Cannot extract the list of managed resources")
		return ctrl.Result{}, err
	}
	instancesStatus := r.StatusClient.GetStatusFromInstances(ctx, instances)
	// Reconcile Pods
	if res, err := r.ReconcilePods(ctx, cluster, database, resources, instancesStatus); err != nil {
		return res, err
	}

	// if len(resources.instances.Items) > 0 && resources.noInstanceIsAlive() {
	// 	return ctrl.Result{RequeueAfter: 1 * time.Second}, r.RegisterPhase(ctx, cluster, apiv1.PhaseUnrecoverable,
	// 		"No pods are active, the cluster needs manual intervention ")
	// }

	// // If we still need more instances, we need to wait before setting healthy status
	// if instancesStatus.InstancesReportingStatus() != cluster.Spec.Instances {
	// 	return ctrl.Result{RequeueAfter: 1 * time.Second}, ErrNextLoop
	// }

	// resources.jobs.

	for idx := range resources.jobs.Items {
		job := &resources.jobs.Items[idx]
		contextLogger.Debug("found a job", "job", job.Name)
	}

	completedJobs := utils.FilterJobsWithOneCompletion(resources.jobs.Items)
	for idx := range completedJobs {
		job := &completedJobs[idx]
		if !job.DeletionTimestamp.IsZero() {
			contextLogger.Debug("skipping job because it has deletion timestamp populated",
				"job", job.Name)
			continue
		}

		contextLogger.Debug("found a completed job", "job", job.Name)
		// if err := r.Delete(ctx, job, &client.DeleteOptions{
		// 	PropagationPolicy: &foreground,
		// }); err != nil {
		// 	contextLogger.Error(err, "cannot delete job", "job", job.Name)
		// 	continue
		// }
	}

	// wait until the database has been created
	if database.Status.Phase != apiv1.DatabasePhaseCompleted {
		return ctrl.Result{RequeueAfter: 1 * time.Second}, ErrNextLoop
	}

	// // When everything is reconciled, update the status
	// if err := r.RegisterPhase(ctx, database, apiv1.DatabasePhaseCompleted, ""); err != nil {
	// 	return ctrl.Result{}, err
	// }

	// r.cleanupCompletedJobs(ctx, resources.jobs)

	return ctrl.Result{}, nil
}

// ReconcilePods decides when to create, scale up/down or wait for pods
func (r *DatabaseReconciler) ReconcilePods(ctx context.Context, cluster *apiv1.Cluster,
	database *apiv1.Database,
	resources *managedDatabaseResources, instancesStatus postgres.PostgresqlStatusList,
) (ctrl.Result, error) {
	// contextLogger := log.FromContext(ctx)

	r.Recorder.Event(database, "Normal", "ReconcilePods", "we should definitely start a job here probs")

	if database.GetStatus().Phase != apiv1.DatabasePhasePending {
		return ctrl.Result{}, nil
	}

	// if err := r.markPVCReadyForCompletedJobs(ctx, resources); err != nil {
	// 	return ctrl.Result{}, err
	// }

	// if res, err := r.ensureInstancesAreCreated(ctx, cluster, resources, instancesStatus); !res.IsZero() || err != nil {
	// 	return res, err
	// }

	// if err := r.ensureHealthyPVCsAnnotation(ctx, cluster, resources); err != nil {
	// 	return ctrl.Result{}, err
	// }

	// // // We have these cases now:
	// // //
	// // // 1 - There is no existent Pod for this PostgreSQL cluster ==> we need to create the
	// // // first node from which we will join the others
	// // //
	// // // 2 - There is one Pod, and that one is still not ready ==> we need to wait
	// // // for the first node to be ready
	// // //
	// // // 3 - We have already some Pods, all they all ready ==> we can create the other
	// // // pods joining the node that we already have.
	// if cluster.Status.Instances == 0 {
	// 	contextLogger.Debug("Waiting for cluster")
	// 	return ctrl.Result{RequeueAfter: 1 * time.Second}, ErrNextLoop
	// }

	// // Stop acting here if there are non-ready Pods unless in maintenance reusing PVCs.
	// // The user have chosen to wait for the missing nodes to come up
	// if instancesStatus.InstancesReportingStatus() < cluster.Status.Instances {
	// 	contextLogger.Debug("Waiting for Pods to be ready")
	// 	return ctrl.Result{RequeueAfter: 1 * time.Second}, ErrNextLoop
	// }

	return r.createDatabase(ctx, cluster, database)

	// // if !instancesStatus.IsComplete() {
	// // 	return ctrl.Result{RequeueAfter: 30 * time.Second}, ErrNextLoop
	// // }

	// // Are there missing nodes? Let's create one
	// if cluster.Status.Instances < cluster.Spec.Instances &&
	// 	instancesStatus.InstancesReportingStatus() == cluster.Status.Instances {
	// 	newNodeSerial, err := r.generateNodeSerial(ctx, cluster)
	// 	if err != nil {
	// 		return ctrl.Result{}, fmt.Errorf("cannot generate node serial: %w", err)
	// 	}
	// 	return r.joinReplicaInstance(ctx, newNodeSerial, cluster)
	// }

	// // Are there nodes to be removed? Remove one of them
	// if cluster.Status.Instances > cluster.Spec.Instances {
	// 	if err := r.scaleDownCluster(ctx, cluster, resources); err != nil {
	// 		return ctrl.Result{}, fmt.Errorf("cannot scale down cluster: %w", err)
	// 	}
	// }

	// // Stop acting here if there are non-ready Pods
	// // In the rest of the function we are sure that
	// // cluster.Status.Instances == cluster.Spec.Instances and
	// // we don't need to modify the cluster topology
	// if cluster.Status.ReadyInstances != cluster.Status.Instances ||
	// 	cluster.Status.ReadyInstances != len(instancesStatus.Items) ||
	// 	!instancesStatus.IsComplete() {
	// 	contextLogger.Debug("Waiting for Pods to be ready")
	// 	return ctrl.Result{RequeueAfter: 1 * time.Second}, ErrNextLoop
	// }

	// return r.handleRollingUpdate(ctx, cluster, instancesStatus)
}

// // getDatabaseTargetPod returns the pod that should run the database setup according to the current
// // cluster's target policy
// func (r *DatabaseReconciler) getDatabaseTargetPod(ctx context.Context,
// 	cluster *apiv1.Cluster,
// 	database *apiv1.Database,
// ) (*corev1.Pod, error) {
// 	var pod corev1.Pod
// 	err := r.Get(ctx, client.ObjectKey{
// 		Namespace: cluster.Namespace,
// 		Name:      cluster.Status.TargetPrimary,
// 	}, &pod)

// 	return &pod, err
// }

// // startCreateDb request a datavase in a Pod and marks the backup started
// // or failed if needed
// func startCreateDb(
// 	ctx context.Context,
// 	client client.Client,
// 	database *apiv1.Database,
// 	pod *corev1.Pod,
// 	cluster *apiv1.Cluster,
// ) error {
// 	// This backup has been started
// 	status := database.GetStatus()
// 	status.SetAsStarted(pod, apiv1.BackupMethodBarmanObjectStore)

// 	if err := postgres.PatchBackupStatusAndRetry(ctx, client, backup); err != nil {
// 		return err
// 	}
// 	config := ctrl.GetConfigOrDie()
// 	clientInterface := kubernetes.NewForConfigOrDie(config)

// 	var err error
// 	var stdout, stderr string
// 	err = retry.OnError(retry.DefaultBackoff, func(error) bool { return true }, func() error {
// 		stdout, stderr, err = utils.ExecCommand(
// 			ctx,
// 			clientInterface,
// 			config,
// 			*pod,
// 			specs.PostgresContainerName,
// 			nil,
// 			"/controller/manager",
// 			"backup",
// 			backup.GetName(),
// 		)
// 		return err
// 	})

// 	if err != nil {
// 		log.FromContext(ctx).Error(err, "executing backup", "stdout", stdout, "stderr", stderr)
// 		status.SetAsFailed(fmt.Errorf("can't execute backup: %w", err))
// 		status.CommandError = stderr
// 		status.CommandError = stdout

// 		// Update backup status in cluster conditions
// 		if errCond := conditions.Patch(ctx, client, cluster, apiv1.BuildClusterBackupFailedCondition(err)); errCond != nil {
// 			log.FromContext(ctx).Error(errCond, "Error while updating backup condition (backup failed)")
// 		}
// 		return postgres.PatchBackupStatusAndRetry(ctx, client, backup)
// 	}

// 	return nil
// }

// SetupWithManager creates a DatabaseReconciler
func (r *DatabaseReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	// err := r.createFieldIndexes(ctx, mgr)
	// if err != nil {
	// 	return err
	// }

	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1.Database{}).
		Owns(&corev1.Pod{}).
		Owns(&batchv1.Job{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.mapSecretsToDatabases()),
			builder.WithPredicates(secretsPredicate),
		).
		Complete(r)
}

// createFieldIndexes creates the indexes needed by this controller
func (r *DatabaseReconciler) createFieldIndexes(ctx context.Context, mgr ctrl.Manager) error {
	// Create a new indexed field on Pods. This field will be used to easily
	// find all the Pods created by this controller
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&corev1.Pod{},
		dbPodOwnerKey, func(rawObj client.Object) []string {
			pod := rawObj.(*corev1.Pod)

			if ownerName, ok := IsOwnedByDatabase(pod); ok {
				return []string{ownerName}
			}

			return nil
		}); err != nil {
		return err
	}

	// Create a new indexed field on Jobs.
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&batchv1.Job{},
		dbJobOwnerKey, func(rawObj client.Object) []string {
			job := rawObj.(*batchv1.Job)

			if ownerName, ok := IsOwnedByDatabase(job); ok {
				return []string{ownerName}
			}

			return nil
		})
}

// IsOwnedByDatabase checks that an object is owned by a Database and returns
// the owner name
func IsOwnedByDatabase(obj client.Object) (string, bool) {
	owner := metav1.GetControllerOf(obj)
	if owner == nil {
		return "", false
	}

	if owner.Kind != apiv1.DatabaseKind {
		return "", false
	}

	if owner.APIVersion != apiGVString {
		return "", false
	}

	return owner.Name, true
}

// mapSecretsToDatabases returns a function mapping catabase events watched to database reconcile requests
func (r *DatabaseReconciler) mapSecretsToDatabases() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		secret, ok := obj.(*corev1.Secret)
		if !ok {
			return nil
		}
		databases, err := r.getDatabasesForSecretsOrConfigMapsToDatabasesMapper(ctx, secret)
		if err != nil {
			log.FromContext(ctx).Error(err, "while getting database list", "namespace", secret.Namespace)
			return nil
		}
		// build requests for catabase referring the secret
		return filterDatabasesUsingSecret(databases, secret)
	}
}

func (r *DatabaseReconciler) getDatabasesForSecretsOrConfigMapsToDatabasesMapper(
	ctx context.Context,
	object metav1.Object,
) (databases apiv1.DatabaseList, err error) {
	_, isSecret := object.(*corev1.Secret)
	_, isConfigMap := object.(*corev1.ConfigMap)

	if !isSecret && !isConfigMap {
		return databases, fmt.Errorf("unsupported object: %+v", object)
	}

	// Get all the clusters handled by the operator in the secret namespaces
	if object.GetNamespace() == configuration.Current.OperatorNamespace &&
		((isConfigMap && object.GetName() == configuration.Current.MonitoringQueriesConfigmap) ||
			(isSecret && object.GetName() == configuration.Current.MonitoringQueriesSecret)) {
		// The events in MonitoringQueriesSecrets impacts all the clusters.
		// We proceed to fetch all the clusters and create a reconciliation request for them.
		// This works as long as the replicated MonitoringQueriesConfigmap in the different namespaces
		// have the same name.
		//
		// See cluster.UsesSecret method
		err = r.List(
			ctx,
			&databases,
			client.MatchingFields{disableDefaultQueriesSpecPath: "false"},
		)
	} else {
		// This is a configmap that affects only a given namespace, so we fetch only the clusters residing in there.
		err = r.List(
			ctx,
			&databases,
			client.InNamespace(object.GetNamespace()),
		)
	}
	return databases, err
}

// // mapNodeToDatabases returns a function mapping cluster events watched to database reconcile requests
// func (r *DatabaseReconciler) mapConfigMapsToDatabases() handler.MapFunc {
// 	return func(ctx context.Context, obj client.Object) []reconcile.Request {
// 		config, ok := obj.(*corev1.ConfigMap)
// 		if !ok {
// 			return nil
// 		}
// 		databases, err := r.getDatabasesForSecretsOrConfigMapsToDatabasesMapper(ctx, config)
// 		if err != nil {
// 			log.FromContext(ctx).Error(err, "while getting database list", "namespace", config.Namespace)
// 			return nil
// 		}
// 		// build requests for databases that refer the configmap
// 		return filterDatabasesUsingConfigMap(databases, config)
// 	}
// }

// filterClustersUsingConfigMap returns a list of reconcile.Request for the clusters
// that reference the secret
func filterDatabasesUsingSecret(
	Databases apiv1.DatabaseList,
	secret *corev1.Secret,
) (requests []reconcile.Request) {
	for _, database := range Databases.Items {
		if database.UsesSecret(secret.Name) {
			requests = append(requests,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      database.Name,
						Namespace: database.Namespace,
					},
				},
			)
			continue
		}
	}
	return requests
}

// // filterDatabasesUsingConfigMap returns a list of reconcile.Request for the Databases
// // that reference the configMap
// func filterDatabasesUsingConfigMap(
// 	Databases apiv1.DatabaseList,
// 	config *corev1.ConfigMap,
// ) (requests []reconcile.Request) {
// 	for _, cluster := range Databases.Items {
// 		if cluster.UsesConfigMap(config.Name) {
// 			requests = append(requests,
// 				reconcile.Request{
// 					NamespacedName: types.NamespacedName{
// 						Name:      cluster.Name,
// 						Namespace: cluster.Namespace,
// 					},
// 				},
// 			)
// 			continue
// 		}
// 	}
// 	return requests
// }

// // mapNodeToClusters returns a function mapping cluster events watched to cluster reconcile requests
// func (r *DatabaseReconciler) mapNodeToClusters() handler.MapFunc {
// 	return func(ctx context.Context, obj client.Object) []reconcile.Request {
// 		node := obj.(*corev1.Node)
// 		// exit if the node is schedulable (e.g. not cordoned)
// 		// could be expanded here with other conditions (e.g. pressure or issues)
// 		if !node.Spec.Unschedulable {
// 			return nil
// 		}
// 		var childPods corev1.PodList
// 		// get all the pods handled by the operator on that node
// 		err := r.List(ctx, &childPods,
// 			client.MatchingFields{".spec.nodeName": node.Name},
// 			client.MatchingLabels{
// 				// TODO: eventually migrate to the new label
// 				utils.ClusterRoleLabelName: specs.ClusterRoleLabelPrimary,
// 				utils.PodRoleLabelName:     string(utils.PodRoleInstance),
// 			},
// 		)
// 		if err != nil {
// 			log.FromContext(ctx).Error(err, "while getting primary instances for node")
// 			return nil
// 		}
// 		var requests []reconcile.Request
// 		// build requests for nodes the pods are running on
// 		for idx := range childPods.Items {
// 			if cluster, ok := IsOwnedByCluster(&childPods.Items[idx]); ok {
// 				requests = append(requests,
// 					reconcile.Request{
// 						NamespacedName: types.NamespacedName{
// 							Name:      cluster,
// 							Namespace: childPods.Items[idx].Namespace,
// 						},
// 					},
// 				)
// 			}
// 		}
// 		return requests
// 	}
// }

// // TODO: only required to cleanup custom monitoring queries configmaps from older versions (v1.10 and v1.11)
// // that could have been copied with the source configmap name instead of the new default one.
// // Should be removed in future releases.
// func (r *DatabaseReconciler) deleteOldCustomQueriesConfigmap(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database) {
// 	contextLogger := log.FromContext(ctx)

// 	// if the cluster didn't have default monitoring queries, do nothing
// 	if cluster.Spec.Monitoring.AreDefaultQueriesDisabled() ||
// 		configuration.Current.MonitoringQueriesConfigmap == "" ||
// 		configuration.Current.MonitoringQueriesConfigmap == apiv1.DefaultMonitoringConfigMapName {
// 		return
// 	}

// 	// otherwise, remove the old default monitoring queries configmap from the cluster and delete it, if present
// 	oldCmID := -1
// 	for idx, cm := range cluster.Spec.Monitoring.CustomQueriesConfigMap {
// 		if cm.Name == configuration.Current.MonitoringQueriesConfigmap &&
// 			cm.Key == apiv1.DefaultMonitoringKey {
// 			oldCmID = idx
// 			break
// 		}
// 	}

// 	// if we didn't find it, do nothing
// 	if oldCmID < 0 {
// 		return
// 	}

// 	// if we found it, we are going to get it and check it was actually created by the operator or was already deleted
// 	var oldCm corev1.ConfigMap
// 	err := r.Get(ctx, types.NamespacedName{
// 		Name:      configuration.Current.MonitoringQueriesConfigmap,
// 		Namespace: cluster.Namespace,
// 	}, &oldCm)
// 	// if we found it, we check the annotation the operator should have set to be sure it was created by us
// 	if err == nil { // nolint:nestif
// 		// if it was, we delete it and proceed to remove it from the cluster monitoring spec
// 		if _, ok := oldCm.Annotations[utils.OperatorVersionAnnotationName]; ok {
// 			err = r.Delete(ctx, &oldCm)
// 			// if there is any error except the cm was already deleted, we return
// 			if err != nil && !apierrs.IsNotFound(err) {
// 				contextLogger.Warning("error while deleting old default monitoring custom queries configmap",
// 					"err", err,
// 					"configmap", configuration.Current.MonitoringQueriesConfigmap)
// 				return
// 			}
// 		} else {
// 			// it exists, but it's not handled by the operator, we do nothing
// 			contextLogger.Warning("A configmap with the same name as the old default monitoring queries "+
// 				"configmap exists, but doesn't have the required annotation, so it won't be deleted, "+
// 				"nor removed from the cluster monitoring spec",
// 				"configmap", oldCm.Name)
// 			return
// 		}
// 	} else if !apierrs.IsNotFound(err) {
// 		// if there is any error except the cm was already deleted, we return
// 		contextLogger.Warning("error while getting old default monitoring custom queries configmap",
// 			"err", err,
// 			"configmap", configuration.Current.MonitoringQueriesConfigmap)
// 		return
// 	}
// 	// both if it exists or not, if we are here we should delete it from the list of custom queries configmaps
// 	oldCluster := cluster.DeepCopy()
// 	cluster.Spec.Monitoring.CustomQueriesConfigMap = append(cluster.Spec.Monitoring.CustomQueriesConfigMap[:oldCmID],
// 		cluster.Spec.Monitoring.CustomQueriesConfigMap[oldCmID+1:]...)
// 	err = r.Patch(ctx, cluster, client.MergeFrom(oldCluster))
// 	if err != nil {
// 		log.Warning("had an error while removing the old custom monitoring queries configmap from "+
// 			"the monitoring section in the cluster",
// 			"err", err,
// 			"configmap", configuration.Current.MonitoringQueriesConfigmap)
// 	}
// }
