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

package controllers

import (
	"context"
	"reflect"
	"sort"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

// managedDatabaseResources contains the resources that are created a cluster
// and need to be managed by the controller
type managedDatabaseResources struct {
	// nodes this is a map composed of [nodeName]corev1.Node
	nodes map[string]corev1.Node
	jobs  batchv1.JobList
}

// Count the number of jobs that are still running
func (resources *managedDatabaseResources) countRunningJobs() int {
	jobCount := len(resources.jobs.Items)
	completeJobs := utils.CountJobsWithOneCompletion(resources.jobs.Items)
	return jobCount - completeJobs
}

// getManagedDatabaseResources get the managed resources of various types
func (r *DatabaseReconciler) getManagedDatabaseResources(
	ctx context.Context,
	cluster *apiv1.Cluster,
	database *apiv1.Database,
) (*managedDatabaseResources, error) {
	childJobs, err := r.getManagedJobs(ctx, cluster, database)
	if err != nil {
		return nil, err
	}

	nodes, err := r.getNodes(ctx)
	if err != nil {
		return nil, err
	}

	return &managedDatabaseResources{
		jobs:  childJobs,
		nodes: nodes,
	}, nil
}

func (r *DatabaseReconciler) getNodes(ctx context.Context) (map[string]corev1.Node, error) {
	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes); err != nil {
		return nil, err
	}

	data := make(map[string]corev1.Node, len(nodes.Items))
	for _, item := range nodes.Items {
		data[item.Name] = item
	}

	return data, nil
}

// getManagedJobs extract the list of jobs which are being created
// by this cluster
func (r *DatabaseReconciler) getManagedJobs(
	ctx context.Context,
	cluster *apiv1.Cluster,
	database *apiv1.Database,
) (batchv1.JobList, error) {
	var childJobs batchv1.JobList
	if err := r.List(ctx, &childJobs,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			utils.ClusterLabelName:  cluster.Name,
			utils.DatabaseLabelName: database.Name,
		},
		client.MatchingFields{jobOwnerKey: database.Name},
	); err != nil {
		return batchv1.JobList{}, err
	}

	sort.Slice(childJobs.Items, func(i, j int) bool {
		return childJobs.Items[i].Name < childJobs.Items[j].Name
	})

	return childJobs, nil
}

func (r *DatabaseReconciler) updateResourceStatus(
	ctx context.Context,
	cluster *apiv1.Cluster,
	database *apiv1.Database,
	resources *managedDatabaseResources,
) error {
	// Retrieve the cluster key

	existingDatabaseStatus := database.Status

	// Count jobs
	newJobs := int32(len(resources.jobs.Items))
	database.Status.JobCount = newJobs

	if err := r.refreshSecretResourceVersions(ctx, database); err != nil {
		return err
	}

	if err := r.refreshConfigMapResourceVersions(ctx, cluster, database); err != nil {
		return err
	}

	if !reflect.DeepEqual(existingDatabaseStatus, database.Status) {
		return r.Status().Update(ctx, database)
	}
	return nil
}

// // removeConditionsWithInvalidReason will remove every condition which has a not valid
// // reason from the K8s API point-of-view
// func (r *DatabaseReconciler) removeConditionsWithInvalidReason(ctx context.Context, database *apiv1.Database) error {
// 	// Nothing to do if cluster has no conditions
// 	if len(database.Status.Conditions) == 0 {
// 		return nil
// 	}

// 	contextLogger := log.FromContext(ctx)
// 	conditions := make([]metav1.Condition, 0, len(database.Status.Conditions))
// 	for _, entry := range database.Status.Conditions {
// 		if utils.IsConditionReasonValid(entry.Reason) {
// 			conditions = append(conditions, entry)
// 		}
// 	}

// 	if !reflect.DeepEqual(database.Status.Conditions, conditions) {
// 		contextLogger.Info("Updating Cluster to remove conditions with invalid reason")
// 		database.Status.Conditions = conditions
// 		if err := r.Status().Update(ctx, database); err != nil {
// 			return err
// 		}

// 		// Restart the reconciliation loop as the status is changed
// 		return ErrNextLoop
// 	}

// 	return nil
// }

// refreshConfigMapResourceVersions set the resource version of the secrets
func (r *DatabaseReconciler) refreshConfigMapResourceVersions(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database) error {
	versions := apiv1.ConfigMapResourceVersion{}

	cluster.Status.ConfigMapResourceVersion = versions

	return nil
}

// refreshSecretResourceVersions set the resource version of the secrets
func (r *DatabaseReconciler) refreshSecretResourceVersions(ctx context.Context, database *apiv1.Database) error {
	versions := apiv1.DbSecretsResourceVersion{}
	var version string
	var err error

	version, err = r.getSecretResourceVersion(ctx, database, database.GetApplicationSecretName())
	if err != nil {
		return err
	}
	versions.ApplicationSecretVersion = version

	if database.ContainsManagedRolesConfiguration() {
		for _, role := range database.Spec.Managed.Roles {
			if role.PasswordSecret != nil {
				version, err = r.getSecretResourceVersion(ctx, database, role.PasswordSecret.Name)
				if err != nil {
					return err
				}
				versions.SetManagedRoleSecretVersion(role.PasswordSecret.Name, &version)
			}
		}
	}

	database.Status.SecretsResourceVersion = versions

	return nil
}

// getSecretResourceVersion retrieves the resource version of a secret
func (r *DatabaseReconciler) getSecretResourceVersion(
	ctx context.Context,
	database *apiv1.Database,
	name string,
) (string, error) {
	return r.getObjectResourceVersion(ctx, database, name, &corev1.Secret{})
}

// getSecretResourceVersion retrieves the resource version of a configmap
func (r *DatabaseReconciler) getConfigMapResourceVersion(
	ctx context.Context,
	database *apiv1.Database,
	name string,
) (string, error) {
	return r.getObjectResourceVersion(ctx, database, name, &corev1.ConfigMap{})
}

// getObjectResourceVersion retrieves the resource version of an object
func (r *DatabaseReconciler) getObjectResourceVersion(
	ctx context.Context,
	database *apiv1.Database,
	name string,
	object client.Object,
) (string, error) {
	err := r.Get(
		ctx,
		client.ObjectKey{Namespace: database.Namespace, Name: name},
		object)
	if err != nil {
		if apierrs.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return object.GetResourceVersion(), nil
}

// RegisterPhase update phase in the status cluster with the
// proper reason
func (r *DatabaseReconciler) RegisterPhase(ctx context.Context,
	database *apiv1.Database,
	phase apiv1.DatabasePhase,
	reason string,
) error {
	// // we ensure that the cluster conditions aren't nil before operating
	// if cluster.Status.Conditions == nil {
	// 	cluster.Status.Conditions = []metav1.Condition{}
	// }

	existingDatabaseStatus := database.Status
	database.Status.Phase = phase
	database.Status.PhaseReason = reason

	// condition := metav1.Condition{
	// 	Type:    string(apiv1.ConditionClusterReady),
	// 	Status:  metav1.ConditionFalse,
	// 	Reason:  string(apiv1.ClusterIsNotReady),
	// 	Message: "Cluster Is Not Ready",
	// }

	// if cluster.Status.Phase == apiv1.PhaseHealthy {
	// 	condition = metav1.Condition{
	// 		Type:    string(apiv1.ConditionClusterReady),
	// 		Status:  metav1.ConditionTrue,
	// 		Reason:  string(apiv1.ClusterReady),
	// 		Message: "Cluster is Ready",
	// 	}
	// }

	// meta.SetStatusCondition(&database.Status.Conditions, condition)

	if !reflect.DeepEqual(existingDatabaseStatus, database.Status) {
		if err := r.Status().Update(ctx, database); err != nil {
			return err
		}
	}

	return nil
}
