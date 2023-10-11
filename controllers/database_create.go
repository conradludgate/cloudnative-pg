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
	"fmt"
	"reflect"
	"time"

	"github.com/sethvargo/go-password/password"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/log"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/resources"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/specs"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/versions"
)

// createPostgresClusterObjects ensures that we have the required global objects
func (r *DatabaseReconciler) createPostgresDatabaseObjects(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database) error {
	err := r.reconcilePostgresSecrets(ctx, cluster, database)
	if err != nil {
		return err
	}

	err = r.createOrPatchServiceAccount(ctx, cluster, database)
	if err != nil {
		return err
	}

	err = r.createOrPatchRole(ctx, cluster, database)
	if err != nil {
		return err
	}

	err = r.createRoleBinding(ctx, database)
	if err != nil {
		return err
	}

	// TODO: only required to cleanup custom monitoring queries configmaps from older versions (v1.10 and v1.11)
	// 		 that could have been copied with the source configmap name instead of the new default one.
	// 		 Should be removed in future releases.
	// should never return an error, not a requirement, just a nice to have
	// r.deleteOldCustomQueriesConfigmap(ctx, cluster, database)

	return nil
}

func (r *DatabaseReconciler) reconcilePostgresSecrets(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database) error {
	return r.reconcileAppUserSecret(ctx, cluster, database)
}

func (r *DatabaseReconciler) reconcileAppUserSecret(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database) error {
	appPassword, err := password.Generate(64, 10, 0, false, true)
	if err != nil {
		return err
	}
	appSecret := specs.CreateSecret(
		database.GetApplicationSecretName(),
		database.Namespace,
		database.GetServiceReadWriteName(),
		database.GetApplicationDatabaseName(),
		database.GetApplicationDatabaseOwner(),
		appPassword)

	database.SetInheritedDataAndOwnership(&appSecret.ObjectMeta)
	if err := resources.CreateIfNotFound(ctx, r.Client, appSecret); err != nil {
		if !apierrs.IsAlreadyExists(err) {
			return err
		}
	}

	return nil
}

// createOrPatchServiceAccount creates or synchronizes the ServiceAccount used by the
// cluster with the latest cluster specification
func (r *DatabaseReconciler) createOrPatchServiceAccount(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database) error {
	var sa corev1.ServiceAccount
	if err := r.Get(ctx, client.ObjectKey{Name: database.Name, Namespace: cluster.Namespace}, &sa); err != nil {
		if !apierrs.IsNotFound(err) {
			return fmt.Errorf("while getting service account: %w", err)
		}

		r.Recorder.Event(database, "Normal", "CreatingServiceAccount", "Creating ServiceAccount")
		return r.createServiceAccount(ctx, cluster, database)
	}

	// generatedPullSecretNames, err := r.generateServiceAccountPullSecretsNames(ctx, cluster, database)
	// if err != nil {
	// 	return fmt.Errorf("while generating pull secret names: %w", err)
	// }

	origSa := sa.DeepCopy()
	// err = specs.UpdateServiceAccount(generatedPullSecretNames, &sa)
	// if err != nil {
	// 	return fmt.Errorf("while generating service account: %w", err)
	// }
	// we add the ownerMetadata only when creating the SA
	database.SetInheritedData(&sa.ObjectMeta)
	database.Spec.ServiceAccountTemplate.MergeMetadata(&sa)

	// if specs.IsServiceAccountAligned(ctx, origSa, generatedPullSecretNames, sa.ObjectMeta) {
	// 	return nil
	// }

	r.Recorder.Event(database, "Normal", "UpdatingServiceAccount", "Updating ServiceAccount")
	if err := r.Patch(ctx, &sa, client.MergeFrom(origSa)); err != nil {
		return fmt.Errorf("while patching service account: %w", err)
	}

	return nil
}

// createServiceAccount creates the service account for this PostgreSQL cluster
func (r *DatabaseReconciler) createServiceAccount(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database) error {
	// generatedPullSecretNames, err := r.generateServiceAccountPullSecretsNames(ctx, cluster, database)
	// if err != nil {
	// 	return fmt.Errorf("while generating pull secret names: %w", err)
	// }

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      database.Name,
		},
	}
	// err = specs.UpdateServiceAccount(generatedPullSecretNames, serviceAccount)
	// if err != nil {
	// 	return fmt.Errorf("while creating new ServiceAccount: %w", err)
	// }

	database.SetInheritedDataAndOwnership(&serviceAccount.ObjectMeta)
	database.Spec.ServiceAccountTemplate.MergeMetadata(serviceAccount)

	err := r.Create(ctx, serviceAccount)
	if err != nil && !apierrs.IsAlreadyExists(err) {
		return err
	}

	return nil
}

// copyPullSecretFromOperator will create a secret to download the operator, if the
// operator was downloaded via a Secret.
// It will return the string of the secret name if a secret need to be used to use the operator
func (r *DatabaseReconciler) copyPullSecretFromOperator(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database) (string, error) {
	if configuration.Current.OperatorNamespace == "" {
		// We are not getting started via a k8s deployment. Perhaps we are running in our development environment
		return "", nil
	}

	// Let's find the operator secret
	var operatorSecret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{
		Name:      configuration.Current.OperatorPullSecretName,
		Namespace: configuration.Current.OperatorNamespace,
	}, &operatorSecret); err != nil {
		if apierrs.IsNotFound(err) {
			// There is no secret like that, probably because we are running in our development environment
			return "", nil
		}
		return "", err
	}

	clusterSecretName := fmt.Sprintf("%s-pull", cluster.Name)

	// Let's create the secret with the required info
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      clusterSecretName,
		},
		Data: operatorSecret.Data,
		Type: operatorSecret.Type,
	}
	cluster.SetInheritedDataAndOwnership(&secret.ObjectMeta)

	// Another sync loop may have already created the service. Let's check that
	if err := r.Create(ctx, &secret); err != nil && !apierrs.IsAlreadyExists(err) {
		return "", err
	}

	return clusterSecretName, nil
}

// createOrPatchRole ensures that the required role for the instance manager exists and
// contains the right rules
func (r *DatabaseReconciler) createOrPatchRole(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database) error {
	originBackup, err := r.getOriginBackup(ctx, cluster, database)
	if err != nil {
		return err
	}

	var role rbacv1.Role
	if err := r.Get(ctx, client.ObjectKey{Name: database.Name, Namespace: cluster.Namespace}, &role); err != nil {
		if !apierrs.IsNotFound(err) {
			return fmt.Errorf("while getting role: %w", err)
		}

		r.Recorder.Event(cluster, "Normal", "CreatingRole", "Creating Cluster Role")
		return r.createRole(ctx, cluster, database, originBackup)
	}

	generatedRole := specs.CreateDbRole(*cluster, *database, originBackup)
	if reflect.DeepEqual(generatedRole.Rules, role.Rules) {
		// Everything fine, the two config maps are exactly the same
		return nil
	}

	r.Recorder.Event(cluster, "Normal", "UpdatingRole", "Updating Cluster Role")

	// The configuration changed, and we need the patch the
	// configMap we have
	patchedRole := role
	patchedRole.Rules = generatedRole.Rules
	if err := r.Patch(ctx, &patchedRole, client.MergeFrom(&role)); err != nil {
		return fmt.Errorf("while patching role: %w", err)
	}

	return nil
}

// createRole creates the role
func (r *DatabaseReconciler) createRole(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database, backupOrigin *apiv1.Backup) error {
	role := specs.CreateDbRole(*cluster, *database, backupOrigin)
	cluster.SetInheritedDataAndOwnership(&role.ObjectMeta)

	err := r.Create(ctx, &role)
	if err != nil && !apierrs.IsAlreadyExists(err) {
		log.FromContext(ctx).Error(err, "Unable to create the Role", "object", role)
		return err
	}

	return nil
}

// createRoleBinding creates the role binding
func (r *DatabaseReconciler) createRoleBinding(ctx context.Context, database *apiv1.Database) error {
	// TODO: fix namespace
	roleBinding := specs.CreateRoleBinding(database.ObjectMeta)
	database.SetInheritedDataAndOwnership(&roleBinding.ObjectMeta)

	err := r.Create(ctx, &roleBinding)
	if err != nil && !apierrs.IsAlreadyExists(err) {
		log.FromContext(ctx).Error(err, "Unable to create the ServiceAccount", "object", roleBinding)
		return err
	}

	return nil
}

// generateNodeSerial extracts the first free node serial in this pods
func (r *DatabaseReconciler) generateNodeSerial(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database) (int, error) {
	cluster.Status.LatestGeneratedNode++
	if err := r.Status().Update(ctx, cluster); err != nil {
		return 0, err
	}

	return cluster.Status.LatestGeneratedNode, nil
}

func (r *DatabaseReconciler) createDatabase(
	ctx context.Context,
	cluster *apiv1.Cluster,
	database *apiv1.Database,
) (ctrl.Result, error) {
	contextLogger := log.FromContext(ctx)

	r.Recorder.Event(cluster, "Normal", "CreatingDatabase", "hopefully creating the database pls")

	// Generate a new node serial
	nodeSerial, err := r.generateNodeSerial(ctx, cluster, database)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("cannot generate node serial: %w", err)
	}

	// We are bootstrapping a cluster and in need to create the first node
	var job *batchv1.Job

	switch {
	case database.Spec.Bootstrap != nil && database.Spec.Bootstrap.Recovery != nil:
		var backup *apiv1.Backup
		if database.Spec.Bootstrap.Recovery.Backup != nil {
			backup, err = r.getOriginBackup(ctx, cluster, database)
			if err != nil {
				return ctrl.Result{}, err
			}
			if backup == nil {
				contextLogger.Info("Missing backup object, can't continue full recovery",
					"backup", database.Spec.Bootstrap.Recovery.Backup)
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: time.Minute,
				}, nil
			}
		}

		panic("todo")

		// if database.Spec.Bootstrap.Recovery.VolumeSnapshots != nil {
		// 	r.Recorder.Event(cluster, "Normal", "CreatingDatabase", "Primary instance (from volumeSnapshots)")
		// 	job = specs.CreatePrimaryJobViaRestoreSnapshot(*cluster, nodeSerial, backup)
		// 	break
		// }

		// r.Recorder.Event(cluster, "Normal", "CreatingDatabase", "Primary instance (from backup)")
		// job = specs.CreatePrimaryJobViaRecovery(*cluster, nodeSerial, backup)
	case cluster.Spec.Bootstrap != nil && cluster.Spec.Bootstrap.PgBaseBackup != nil:
		panic("todo")
		// r.Recorder.Event(cluster, "Normal", "CreatingDatabase", "Primary instance (from physical backup)")
		// job = specs.CreatePrimaryJobViaPgBaseBackup(*cluster, nodeSerial)
	default:
		r.Recorder.Event(cluster, "Normal", "CreatingDatabase", "Primary instance (initdb)")
		job = specs.CreateDatabaseSetupJob(*cluster, *database, nodeSerial)
	}

	if err := ctrl.SetControllerReference(database, job, r.Scheme); err != nil {
		contextLogger.Error(err, "Unable to set the owner reference for instance")
		return ctrl.Result{}, err
	}

	err = r.RegisterPhase(ctx, database, apiv1.DatabasePhaseStarted,
		fmt.Sprintf("Creating database job %v", job.Name))
	if err != nil {
		return ctrl.Result{}, err
	}

	contextLogger.Info("Creating new Job",
		"name", job.Name,
		"primary", true)

	utils.SetOperatorVersion(&job.ObjectMeta, versions.Version)
	utils.InheritAnnotations(&job.ObjectMeta, database.Annotations,
		database.GetFixedInheritedAnnotations(), configuration.Current)
	utils.InheritAnnotations(&job.Spec.Template.ObjectMeta, database.Annotations,
		database.GetFixedInheritedAnnotations(), configuration.Current)
	utils.InheritLabels(&job.ObjectMeta, database.Labels,
		database.GetFixedInheritedLabels(), configuration.Current)
	utils.InheritLabels(&job.Spec.Template.ObjectMeta, database.Labels,
		database.GetFixedInheritedLabels(), configuration.Current)

	if err = r.Create(ctx, job); err != nil {
		if apierrs.IsAlreadyExists(err) {
			// This Job was already created, maybe the cache is stale.
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		contextLogger.Error(err, "Unable to create job", "job", job)
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, ErrNextLoop
}

// getOriginBackup gets the backup that is used to bootstrap a new PostgreSQL cluster
func (r *DatabaseReconciler) getOriginBackup(ctx context.Context, cluster *apiv1.Cluster, database *apiv1.Database) (*apiv1.Backup, error) {
	if database.Spec.Bootstrap == nil ||
		database.Spec.Bootstrap.Recovery == nil ||
		database.Spec.Bootstrap.Recovery.Backup == nil {
		return nil, nil
	}

	var backup apiv1.Backup
	backupObjectKey := client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      database.Spec.Bootstrap.Recovery.Backup.Name,
	}
	err := r.Get(ctx, backupObjectKey, &backup)
	if err != nil {
		if apierrs.IsNotFound(err) {
			r.Recorder.Eventf(database, "Warning", "ErrorNoBackup",
				"Backup object \"%v/%v\" is missing",
				backupObjectKey.Namespace, backupObjectKey.Name)

			return nil, nil
		}

		return nil, fmt.Errorf("cannot get the backup object: %w", err)
	}

	return &backup, nil
}
