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

package v1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/system"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/versions"
)

// const (
// 	// ApplicationUserSecretSuffix is the suffix appended to the cluster name to
// 	// get the name of the application user secret
// 	ApplicationUserSecretSuffix = "-app"
// )

// DatabaseSpec defines the desired state of Database
type DatabaseSpec struct {
	// The cluster to add this database on
	Cluster GlobalObjectReference `json:"cluster"`

	// Description of this PostgreSQL database
	// +optional
	Description string `json:"description,omitempty"`

	// Metadata that will be inherited by all objects related to the Cluster
	// +optional
	InheritedMetadata *EmbeddedObjectMetadata `json:"inheritedMetadata,omitempty"`

	// // Name of the container image, supporting both tags (`<image>:<tag>`)
	// // and digests for deterministic and repeatable deployments
	// // (`<image>:<tag>@sha256:<digestValue>`)
	// // +optional
	// ImageName string `json:"imageName,omitempty"`

	// // Image pull policy.
	// // One of `Always`, `Never` or `IfNotPresent`.
	// // If not defined, it defaults to `IfNotPresent`.
	// // Cannot be updated.
	// // More info: https://kubernetes.io/docs/concepts/containers/images#updating-images
	// // +optional
	// ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// // If specified, the pod will be dispatched by specified Kubernetes
	// // scheduler. If not specified, the pod will be dispatched by the default
	// // scheduler. More info:
	// // https://kubernetes.io/docs/concepts/scheduling-eviction/kube-scheduler/
	// // +optional
	// SchedulerName string `json:"schedulerName,omitempty"`

	// Instructions to bootstrap this cluster
	// +optional
	Bootstrap *BootstrapConfiguration `json:"bootstrap,omitempty"`

	// Configure the generation of the service account
	// +optional
	ServiceAccountTemplate *ServiceAccountTemplate `json:"serviceAccountTemplate,omitempty"`

	// // The configuration to be used for backups
	// // +optional
	// Backup *BackupConfiguration `json:"backup,omitempty"`

	// The configuration of the monitoring infrastructure of this cluster
	// +optional
	Monitoring *MonitoringConfiguration `json:"monitoring,omitempty"`

	// The list of external clusters which are used in the configuration
	// +optional
	ExternalClusters []ExternalCluster `json:"externalClusters,omitempty"`

	// The instances' log level, one of the following values: error, warning, info (default), debug, trace
	// +kubebuilder:default:=info
	// +kubebuilder:validation:Enum:=error;warning;info;debug;trace
	// +optional
	LogLevel string `json:"logLevel,omitempty"`

	// The configuration that is used by the portions of PostgreSQL that are managed by the instance manager
	// +optional
	Managed *ManagedConfiguration `json:"managed,omitempty"`
}

// // ServiceAccountTemplate contains the template needed to generate the service accounts
// type ServiceAccountTemplate struct {
// 	// Metadata are the metadata to be used for the generated
// 	// service account
// 	Metadata Metadata `json:"metadata"`
// }

// // MergeMetadata adds the passed custom annotations and labels in the service account.
// func (st *ServiceAccountTemplate) MergeMetadata(sa *corev1.ServiceAccount) {
// 	if st == nil {
// 		return
// 	}
// 	if sa.Labels == nil {
// 		sa.Labels = map[string]string{}
// 	}
// 	if sa.Annotations == nil {
// 		sa.Annotations = map[string]string{}
// 	}

// 	utils.MergeMap(sa.Labels, st.Metadata.Labels)
// 	utils.MergeMap(sa.Annotations, st.Metadata.Annotations)
// }

// // RoleStatus represents the status of a managed role in the cluster
// type RoleStatus string

// const (
// 	// RoleStatusReconciled indicates the role in DB matches the Spec
// 	RoleStatusReconciled RoleStatus = "reconciled"
// 	// RoleStatusNotManaged indicates the role is not in the Spec, therefore not managed
// 	RoleStatusNotManaged RoleStatus = "not-managed"
// 	// RoleStatusPendingReconciliation indicates the role in Spec requires updated/creation in DB
// 	RoleStatusPendingReconciliation RoleStatus = "pending-reconciliation"
// 	// RoleStatusReserved indicates this is one of the roles reserved by the operator. E.g. `postgres`
// 	RoleStatusReserved RoleStatus = "reserved"
// )

// // PasswordState represents the state of the password of a managed RoleConfiguration
// type PasswordState struct {
// 	// the last transaction ID to affect the role definition in PostgreSQL
// 	// +optional
// 	TransactionID int64 `json:"transactionID,omitempty"`
// 	// the resource version of the password secret
// 	// +optional
// 	SecretResourceVersion string `json:"resourceVersion,omitempty"`
// }

// // ManagedRoles tracks the status of a cluster's managed roles
// type ManagedRoles struct {
// 	// ByStatus gives the list of roles in each state
// 	// +optional
// 	ByStatus map[RoleStatus][]string `json:"byStatus,omitempty"`

// 	// CannotReconcile lists roles that cannot be reconciled in PostgreSQL,
// 	// with an explanation of the cause
// 	// +optional
// 	CannotReconcile map[string][]string `json:"cannotReconcile,omitempty"`

// 	// PasswordStatus gives the last transaction id and password secret version for each managed role
// 	// +optional
// 	PasswordStatus map[string]PasswordState `json:"passwordStatus,omitempty"`
// }

// DatabaseStatus defines the observed state of Database
type DatabaseStatus struct {
	// ManagedRolesStatus reports the state of the managed roles in the cluster
	// +optional
	ManagedRolesStatus ManagedRoles `json:"managedRolesStatus,omitempty"`

	// The list of resource versions of the secrets
	// managed by the operator. Every change here is done in the
	// interest of the instance manager, which will refresh the
	// secret data
	// +optional
	SecretsResourceVersion DbSecretsResourceVersion `json:"secretsResourceVersion,omitempty"`
}

// // BootstrapConfiguration contains information about how to create the PostgreSQL
// // cluster. Only a single bootstrap method can be defined among the supported
// // ones. `initdb` will be used as the bootstrap method if left
// // unspecified. Refer to the Bootstrap page of the documentation for more
// // information.
// type BootstrapConfiguration struct {
// 	// Bootstrap the cluster via initdb
// 	// +optional
// 	InitDB *BootstrapInitDB `json:"initdb,omitempty"`

// 	// Bootstrap the cluster from a backup
// 	// +optional
// 	Recovery *BootstrapRecovery `json:"recovery,omitempty"`

// 	// Bootstrap the cluster taking a physical backup of another compatible
// 	// PostgreSQL instance
// 	// +optional
// 	PgBaseBackup *BootstrapPgBaseBackup `json:"pg_basebackup,omitempty"`
// }

// // BootstrapInitDB is the configuration of the bootstrap process when
// // initdb is used
// // Refer to the Bootstrap page of the documentation for more information.
// type BootstrapInitDB struct {
// 	// Name of the database used by the application. Default: `app`.
// 	// +optional
// 	Database string `json:"database,omitempty"`

// 	// Name of the owner of the database in the instance to be used
// 	// by applications. Defaults to the value of the `database` key.
// 	// +optional
// 	Owner string `json:"owner,omitempty"`

// 	// Name of the secret containing the initial credentials for the
// 	// owner of the user database. If empty a new secret will be
// 	// created from scratch
// 	// +optional
// 	Secret *LocalObjectReference `json:"secret,omitempty"`

// 	// The list of options that must be passed to initdb when creating the cluster.
// 	// Deprecated: This could lead to inconsistent configurations,
// 	// please use the explicit provided parameters instead.
// 	// If defined, explicit values will be ignored.
// 	// +optional
// 	Options []string `json:"options,omitempty"`

// 	// Whether the `-k` option should be passed to initdb,
// 	// enabling checksums on data pages (default: `false`)
// 	// +optional
// 	DataChecksums *bool `json:"dataChecksums,omitempty"`

// 	// The value to be passed as option `--encoding` for initdb (default:`UTF8`)
// 	// +optional
// 	Encoding string `json:"encoding,omitempty"`

// 	// The value to be passed as option `--lc-collate` for initdb (default:`C`)
// 	// +optional
// 	LocaleCollate string `json:"localeCollate,omitempty"`

// 	// The value to be passed as option `--lc-ctype` for initdb (default:`C`)
// 	// +optional
// 	LocaleCType string `json:"localeCType,omitempty"`

// 	// The value in megabytes (1 to 1024) to be passed to the `--wal-segsize`
// 	// option for initdb (default: empty, resulting in PostgreSQL default: 16MB)
// 	// +kubebuilder:validation:Minimum=1
// 	// +kubebuilder:validation:Maximum=1024
// 	// +optional
// 	WalSegmentSize int `json:"walSegmentSize,omitempty"`

// 	// List of SQL queries to be executed as a superuser immediately
// 	// after the cluster has been created - to be used with extreme care
// 	// (by default empty)
// 	// +optional
// 	PostInitSQL []string `json:"postInitSQL,omitempty"`

// 	// List of SQL queries to be executed as a superuser in the application
// 	// database right after is created - to be used with extreme care
// 	// (by default empty)
// 	// +optional
// 	PostInitApplicationSQL []string `json:"postInitApplicationSQL,omitempty"`

// 	// List of SQL queries to be executed as a superuser in the `template1`
// 	// after the cluster has been created - to be used with extreme care
// 	// (by default empty)
// 	// +optional
// 	PostInitTemplateSQL []string `json:"postInitTemplateSQL,omitempty"`

// 	// Bootstraps the new cluster by importing data from an existing PostgreSQL
// 	// instance using logical backup (`pg_dump` and `pg_restore`)
// 	// +optional
// 	Import *Import `json:"import,omitempty"`

// 	// PostInitApplicationSQLRefs points references to ConfigMaps or Secrets which
// 	// contain SQL files, the general implementation order to these references is
// 	// from all Secrets to all ConfigMaps, and inside Secrets or ConfigMaps,
// 	// the implementation order is same as the order of each array
// 	// (by default empty)
// 	// +optional
// 	PostInitApplicationSQLRefs *PostInitApplicationSQLRefs `json:"postInitApplicationSQLRefs,omitempty"`
// }

// // Import contains the configuration to init a database from a logic snapshot of an externalCluster
// type Import struct {
// 	// The source of the import
// 	Source ImportSource `json:"source"`

// 	// The import type. Can be `microservice` or `monolith`.
// 	// +kubebuilder:validation:Enum=microservice;monolith
// 	Type SnapshotType `json:"type"`

// 	// The databases to import
// 	Databases []string `json:"databases"`

// 	// The roles to import
// 	// +optional
// 	Roles []string `json:"roles,omitempty"`

// 	// List of SQL queries to be executed as a superuser in the application
// 	// database right after is imported - to be used with extreme care
// 	// (by default empty). Only available in microservice type.
// 	// +optional
// 	PostImportApplicationSQL []string `json:"postImportApplicationSQL,omitempty"`

// 	// When set to true, only the `pre-data` and `post-data` sections of
// 	// `pg_restore` are invoked, avoiding data import. Default: `false`.
// 	// +kubebuilder:default:=false
// 	// +optional
// 	SchemaOnly bool `json:"schemaOnly,omitempty"`
// }

// // ImportSource describes the source for the logical snapshot
// type ImportSource struct {
// 	// The name of the externalCluster used for import
// 	ExternalCluster string `json:"externalCluster"`
// }

// // PostInitApplicationSQLRefs points references to ConfigMaps or Secrets which
// // contain SQL files, the general implementation order to these references is
// // from all Secrets to all ConfigMaps, and inside Secrets or ConfigMaps,
// // the implementation order is same as the order of each array
// type PostInitApplicationSQLRefs struct {
// 	// SecretRefs holds a list of references to Secrets
// 	// +optional
// 	SecretRefs []SecretKeySelector `json:"secretRefs,omitempty"`

// 	// ConfigMapRefs holds a list of references to ConfigMaps
// 	// +optional
// 	ConfigMapRefs []ConfigMapKeySelector `json:"configMapRefs,omitempty"`
// }

// // BootstrapRecovery contains the configuration required to restore
// // from an existing cluster using 3 methodologies: external cluster,
// // volume snapshots or backup objects. Full recovery and Point-In-Time
// // Recovery are supported.
// // The method can be also be used to create clusters in continuous recovery
// // (replica clusters), also supporting cascading replication when `instances` >
// // 1. Once the cluster exits recovery, the password for the superuser
// // will be changed through the provided secret.
// // Refer to the Bootstrap page of the documentation for more information.
// type BootstrapRecovery struct {
// 	// The backup object containing the physical base backup from which to
// 	// initiate the recovery procedure.
// 	// Mutually exclusive with `source` and `volumeSnapshots`.
// 	// +optional
// 	Backup *BackupSource `json:"backup,omitempty"`

// 	// The external cluster whose backup we will restore. This is also
// 	// used as the name of the folder under which the backup is stored,
// 	// so it must be set to the name of the source cluster
// 	// Mutually exclusive with `backup`.
// 	// +optional
// 	Source string `json:"source,omitempty"`

// 	// The static PVC data source(s) from which to initiate the
// 	// recovery procedure. Currently supporting `VolumeSnapshot`
// 	// and `PersistentVolumeClaim` resources that map an existing
// 	// PVC group, compatible with CloudNativePG, and taken with
// 	// a cold backup copy on a fenced Postgres instance (limitation
// 	// which will be removed in the future when online backup
// 	// will be implemented).
// 	// Mutually exclusive with `backup`.
// 	// +optional
// 	VolumeSnapshots *DataSource `json:"volumeSnapshots,omitempty"`

// 	// By default, the recovery process applies all the available
// 	// WAL files in the archive (full recovery). However, you can also
// 	// end the recovery as soon as a consistent state is reached or
// 	// recover to a point-in-time (PITR) by specifying a `RecoveryTarget` object,
// 	// as expected by PostgreSQL (i.e., timestamp, transaction Id, LSN, ...).
// 	// More info: https://www.postgresql.org/docs/current/runtime-config-wal.html#RUNTIME-CONFIG-WAL-RECOVERY-TARGET
// 	// +optional
// 	RecoveryTarget *RecoveryTarget `json:"recoveryTarget,omitempty"`

// 	// Name of the database used by the application. Default: `app`.
// 	// +optional
// 	Database string `json:"database,omitempty"`

// 	// Name of the owner of the database in the instance to be used
// 	// by applications. Defaults to the value of the `database` key.
// 	// +optional
// 	Owner string `json:"owner,omitempty"`

// 	// Name of the secret containing the initial credentials for the
// 	// owner of the user database. If empty a new secret will be
// 	// created from scratch
// 	// +optional
// 	Secret *LocalObjectReference `json:"secret,omitempty"`
// }

// // DataSource contains the configuration required to bootstrap a
// // PostgreSQL cluster from an existing storage
// type DataSource struct {
// 	// Configuration of the storage of the instances
// 	Storage corev1.TypedLocalObjectReference `json:"storage"`

// 	// Configuration of the storage for PostgreSQL WAL (Write-Ahead Log)
// 	// +optional
// 	WalStorage *corev1.TypedLocalObjectReference `json:"walStorage,omitempty"`
// }

// // BackupSource contains the backup we need to restore from, plus some
// // information that could be needed to correctly restore it.
// type BackupSource struct {
// 	LocalObjectReference `json:",inline"`
// 	// EndpointCA store the CA bundle of the barman endpoint.
// 	// Useful when using self-signed certificates to avoid
// 	// errors with certificate issuer and barman-cloud-wal-archive.
// 	// +optional
// 	EndpointCA *SecretKeySelector `json:"endpointCA,omitempty"`
// }

// // BootstrapPgBaseBackup contains the configuration required to take
// // a physical backup of an existing PostgreSQL cluster
// type BootstrapPgBaseBackup struct {
// 	// The name of the server of which we need to take a physical backup
// 	// +kubebuilder:validation:MinLength=1
// 	Source string `json:"source"`

// 	// Name of the database used by the application. Default: `app`.
// 	// +optional
// 	Database string `json:"database,omitempty"`

// 	// Name of the owner of the database in the instance to be used
// 	// by applications. Defaults to the value of the `database` key.
// 	// +optional
// 	Owner string `json:"owner,omitempty"`

// 	// Name of the secret containing the initial credentials for the
// 	// owner of the user database. If empty a new secret will be
// 	// created from scratch
// 	// +optional
// 	Secret *LocalObjectReference `json:"secret,omitempty"`
// }

// // RecoveryTarget allows to configure the moment where the recovery process
// // will stop. All the target options except TargetTLI are mutually exclusive.
// type RecoveryTarget struct {
// 	// The ID of the backup from which to start the recovery process.
// 	// If empty (default) the operator will automatically detect the backup
// 	// based on targetTime or targetLSN if specified. Otherwise use the
// 	// latest available backup in chronological order.
// 	// +optional
// 	BackupID string `json:"backupID,omitempty"`

// 	// The target timeline ("latest" or a positive integer)
// 	// +optional
// 	TargetTLI string `json:"targetTLI,omitempty"`

// 	// The target transaction ID
// 	// +optional
// 	TargetXID string `json:"targetXID,omitempty"`

// 	// The target name (to be previously created
// 	// with `pg_create_restore_point`)
// 	// +optional
// 	TargetName string `json:"targetName,omitempty"`

// 	// The target LSN (Log Sequence Number)
// 	// +optional
// 	TargetLSN string `json:"targetLSN,omitempty"`

// 	// The target time as a timestamp in the RFC3339 standard
// 	// +optional
// 	TargetTime string `json:"targetTime,omitempty"`

// 	// End recovery as soon as a consistent state is reached
// 	// +optional
// 	TargetImmediate *bool `json:"targetImmediate,omitempty"`

// 	// Set the target to be exclusive. If omitted, defaults to false, so that
// 	// in Postgres, `recovery_target_inclusive` will be true
// 	// +optional
// 	Exclusive *bool `json:"exclusive,omitempty"`
// }

// // ExternalCluster represents the connection parameters to an
// // external cluster which is used in the other sections of the configuration
// type ExternalCluster struct {
// 	// The server name, required
// 	Name string `json:"name"`

// 	// The list of connection parameters, such as dbname, host, username, etc
// 	// +optional
// 	ConnectionParameters map[string]string `json:"connectionParameters,omitempty"`

// 	// The reference to an SSL certificate to be used to connect to this
// 	// instance
// 	// +optional
// 	SSLCert *corev1.SecretKeySelector `json:"sslCert,omitempty"`

// 	// The reference to an SSL private key to be used to connect to this
// 	// instance
// 	// +optional
// 	SSLKey *corev1.SecretKeySelector `json:"sslKey,omitempty"`

// 	// The reference to an SSL CA public key to be used to connect to this
// 	// instance
// 	// +optional
// 	SSLRootCert *corev1.SecretKeySelector `json:"sslRootCert,omitempty"`

// 	// The reference to the password to be used to connect to the server
// 	// +optional
// 	Password *corev1.SecretKeySelector `json:"password,omitempty"`

// 	// The configuration for the barman-cloud tool suite
// 	// +optional
// 	BarmanObjectStore *BarmanObjectStoreConfiguration `json:"barmanObjectStore,omitempty"`
// }

// // GetServerName returns the server name, defaulting to the name of the external cluster or using the one specified
// // in the BarmanObjectStore
// func (in ExternalCluster) GetServerName() string {
// 	if in.BarmanObjectStore != nil && in.BarmanObjectStore.ServerName != "" {
// 		return in.BarmanObjectStore.ServerName
// 	}
// 	return in.Name
// }

// // ManagedConfiguration represents the portions of PostgreSQL that are managed
// // by the instance manager
// type ManagedConfiguration struct {
// 	// Database roles managed by the `Cluster`
// 	// +optional
// 	Roles []RoleConfiguration `json:"roles,omitempty"`
// }

// // RoleConfiguration is the representation, in Kubernetes, of a PostgreSQL role
// // with the additional field Ensure specifying whether to ensure the presence or
// // absence of the role in the database
// //
// // The defaults of the CREATE ROLE command are applied
// // Reference: https://www.postgresql.org/docs/current/sql-createrole.html
// type RoleConfiguration struct {
// 	// Name of the role
// 	Name string `json:"name"`
// 	// Description of the role
// 	// +optional
// 	Comment string `json:"comment,omitempty"`

// 	// Ensure the role is `present` or `absent` - defaults to "present"
// 	// +kubebuilder:default:="present"
// 	// +kubebuilder:validation:Enum=present;absent
// 	// +optional
// 	Ensure EnsureOption `json:"ensure,omitempty"`

// 	// Secret containing the password of the role (if present)
// 	// If null, the password will be ignored unless DisablePassword is set
// 	// +optional
// 	PasswordSecret *LocalObjectReference `json:"passwordSecret,omitempty"`

// 	// If the role can log in, this specifies how many concurrent
// 	// connections the role can make. `-1` (the default) means no limit.
// 	// +kubebuilder:default:=-1
// 	// +optional
// 	ConnectionLimit int64 `json:"connectionLimit,omitempty"`

// 	// Date and time after which the role's password is no longer valid.
// 	// When omitted, the password will never expire (default).
// 	// +optional
// 	ValidUntil *metav1.Time `json:"validUntil,omitempty"`

// 	// List of one or more existing roles to which this role will be
// 	// immediately added as a new member. Default empty.
// 	// +optional
// 	InRoles []string `json:"inRoles,omitempty"`

// 	// Whether a role "inherits" the privileges of roles it is a member of.
// 	// Defaults is `true`.
// 	// +kubebuilder:default:=true
// 	// +optional
// 	Inherit *bool `json:"inherit,omitempty"` // IMPORTANT default is INHERIT

// 	// DisablePassword indicates that a role's password should be set to NULL in Postgres
// 	// +optional
// 	DisablePassword bool `json:"disablePassword,omitempty"`

// 	// Whether the role is a `superuser` who can override all access
// 	// restrictions within the database - superuser status is dangerous and
// 	// should be used only when really needed. You must yourself be a
// 	// superuser to create a new superuser. Defaults is `false`.
// 	// +optional
// 	Superuser bool `json:"superuser,omitempty"`

// 	// When set to `true`, the role being defined will be allowed to create
// 	// new databases. Specifying `false` (default) will deny a role the
// 	// ability to create databases.
// 	// +optional
// 	CreateDB bool `json:"createdb,omitempty"`

// 	// Whether the role will be permitted to create, alter, drop, comment
// 	// on, change the security label for, and grant or revoke membership in
// 	// other roles. Default is `false`.
// 	// +optional
// 	CreateRole bool `json:"createrole,omitempty"`

// 	// Whether the role is allowed to log in. A role having the `login`
// 	// attribute can be thought of as a user. Roles without this attribute
// 	// are useful for managing database privileges, but are not users in
// 	// the usual sense of the word. Default is `false`.
// 	// +optional
// 	Login bool `json:"login,omitempty"`

// 	// Whether a role is a replication role. A role must have this
// 	// attribute (or be a superuser) in order to be able to connect to the
// 	// server in replication mode (physical or logical replication) and in
// 	// order to be able to create or drop replication slots. A role having
// 	// the `replication` attribute is a very highly privileged role, and
// 	// should only be used on roles actually used for replication. Default
// 	// is `false`.
// 	// +optional
// 	Replication bool `json:"replication,omitempty"`

// 	// Whether a role bypasses every row-level security (RLS) policy.
// 	// Default is `false`.
// 	// +optional
// 	BypassRLS bool `json:"bypassrls,omitempty"` // Row-Level Security
// }

// // GetRoleSecretsName gets the name of the secret which is used to store the role's password
// func (roleConfiguration *RoleConfiguration) GetRoleSecretsName() string {
// 	if roleConfiguration.PasswordSecret != nil {
// 		return roleConfiguration.PasswordSecret.Name
// 	}
// 	return ""
// }

// // GetRoleInherit return the inherit attribute of a roleConfiguration
// func (roleConfiguration *RoleConfiguration) GetRoleInherit() bool {
// 	if roleConfiguration.Inherit != nil {
// 		return *roleConfiguration.Inherit
// 	}
// 	return true
// }

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.instances,statuspath=.status.instances
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Instances",type="integer",JSONPath=".status.instances",description="Number of instances"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyInstances",description="Number of ready instances"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="Cluster current status"
// +kubebuilder:printcolumn:name="Primary",type="string",JSONPath=".status.currentPrimary",description="Primary pod"

// Database is the Schema for the PostgreSQL API
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	// Specification of the desired behavior of the cluster.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	Spec DatabaseSpec `json:"spec"`
	// Most recently observed status of the cluster. This data may not be up
	// to date. Populated by the system. Read-only.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status DatabaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseList contains a list of Database
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of databases
	Items []Database `json:"items"`
}

// SecretsResourceVersion is the resource versions of the secrets
// managed by the operator
type DbSecretsResourceVersion struct {
	// The resource version of the "app" user secret
	// +optional
	ApplicationSecretVersion string `json:"applicationSecretVersion,omitempty"`

	// The resource versions of the managed roles secrets
	// +optional
	ManagedRoleSecretVersions map[string]string `json:"managedRoleSecretVersion,omitempty"`
}

// SetManagedRoleSecretVersion Add or update or delete the resource version of the managed role secret
func (secretResourceVersion *DbSecretsResourceVersion) SetManagedRoleSecretVersion(secret string, version *string) {
	if secretResourceVersion.ManagedRoleSecretVersions == nil {
		secretResourceVersion.ManagedRoleSecretVersions = make(map[string]string)
	}
	if version == nil {
		delete(secretResourceVersion.ManagedRoleSecretVersions, secret)
	} else {
		secretResourceVersion.ManagedRoleSecretVersions[secret] = *version
	}
}

// // GetImageName get the name of the image that should be used
// // to create the pods
// func (cluster *Database) GetImageName() string {
// 	if len(cluster.Spec.ImageName) > 0 {
// 		return cluster.Spec.ImageName
// 	}

// 	return configuration.Current.PostgresImageName
// }

// ContainsManagedRolesConfiguration returns true iff there are managed roles configured
func (cluster *Database) ContainsManagedRolesConfiguration() bool {
	return cluster.Spec.Managed != nil && len(cluster.Spec.Managed.Roles) > 0
}

// UsesSecretInManagedRoles checks if the given secret name is used in a managed role
func (cluster *Database) UsesSecretInManagedRoles(secretName string) bool {
	if !cluster.ContainsManagedRolesConfiguration() {
		return false
	}
	for _, role := range cluster.Spec.Managed.Roles {
		if role.PasswordSecret != nil && role.PasswordSecret.Name == secretName {
			return true
		}
	}
	return false
}

// GetApplicationSecretName get the name of the application secret for any bootstrap type
func (cluster *Database) GetApplicationSecretName() string {
	bootstrap := cluster.Spec.Bootstrap
	if bootstrap == nil {
		return fmt.Sprintf("%v%v", cluster.Name, ApplicationUserSecretSuffix)
	}
	recovery := bootstrap.Recovery
	if recovery != nil && recovery.Secret != nil && recovery.Secret.Name != "" {
		return recovery.Secret.Name
	}

	pgBaseBackup := bootstrap.PgBaseBackup
	if pgBaseBackup != nil && pgBaseBackup.Secret != nil && pgBaseBackup.Secret.Name != "" {
		return pgBaseBackup.Secret.Name
	}

	initDB := bootstrap.InitDB
	if initDB != nil && initDB.Secret != nil && initDB.Secret.Name != "" {
		return initDB.Secret.Name
	}

	return fmt.Sprintf("%v%v", cluster.Name, ApplicationUserSecretSuffix)
}

// GetApplicationDatabaseName get the name of the application database for a specific bootstrap
func (cluster *Database) GetApplicationDatabaseName() string {
	bootstrap := cluster.Spec.Bootstrap
	if bootstrap == nil {
		return ""
	}

	if bootstrap.Recovery != nil && bootstrap.Recovery.Database != "" {
		return bootstrap.Recovery.Database
	}

	if bootstrap.PgBaseBackup != nil && bootstrap.PgBaseBackup.Database != "" {
		return bootstrap.PgBaseBackup.Database
	}

	if bootstrap.InitDB != nil && bootstrap.InitDB.Database != "" {
		return bootstrap.InitDB.Database
	}

	return ""
}

// GetApplicationDatabaseOwner get the owner user of the application database for a specific bootstrap
func (cluster *Database) GetApplicationDatabaseOwner() string {
	bootstrap := cluster.Spec.Bootstrap
	if bootstrap == nil {
		return ""
	}

	if bootstrap.Recovery != nil && bootstrap.Recovery.Owner != "" {
		return bootstrap.Recovery.Owner
	}

	if bootstrap.PgBaseBackup != nil && bootstrap.PgBaseBackup.Owner != "" {
		return bootstrap.PgBaseBackup.Owner
	}

	if bootstrap.InitDB != nil && bootstrap.InitDB.Owner != "" {
		return bootstrap.InitDB.Owner
	}

	return ""
}

// GetFixedInheritedAnnotations gets the annotations that should be
// inherited by all resources according the cluster spec
func (cluster *Database) GetFixedInheritedAnnotations() map[string]string {
	if cluster.Spec.InheritedMetadata == nil || cluster.Spec.InheritedMetadata.Annotations == nil {
		return nil
	}
	return cluster.Spec.InheritedMetadata.Annotations
}

// GetFixedInheritedLabels gets the labels that should be
// inherited by all resources according the cluster spec
func (cluster *Database) GetFixedInheritedLabels() map[string]string {
	if cluster.Spec.InheritedMetadata == nil || cluster.Spec.InheritedMetadata.Labels == nil {
		return nil
	}
	return cluster.Spec.InheritedMetadata.Labels
}

// GetServiceAnyName return the name of the service that is used as DNS
// domain for all the nodes, even if they are not ready
func (cluster *Database) GetServiceAnyName() string {
	return fmt.Sprintf("%v%v", cluster.Name, ServiceAnySuffix)
}

// GetServiceReadName return the name of the service that is used for
// read transactions (including the primary)
func (cluster *Database) GetServiceReadName() string {
	return fmt.Sprintf("%v%v", cluster.Name, ServiceReadSuffix)
}

// GetServiceReadOnlyName return the name of the service that is used for
// read-only transactions (excluding the primary)
func (cluster *Database) GetServiceReadOnlyName() string {
	return fmt.Sprintf("%v%v", cluster.Name, ServiceReadOnlySuffix)
}

// GetServiceReadWriteName return the name of the service that is used for
// read-write transactions
func (cluster *Database) GetServiceReadWriteName() string {
	return fmt.Sprintf("%v%v", cluster.Name, ServiceReadWriteSuffix)
}

// IsInstanceFenced check if in a given instance should be fenced
func (cluster *Database) IsInstanceFenced(instance string) bool {
	fencedInstances, err := utils.GetFencedInstances(cluster.Annotations)
	if err != nil {
		return false
	}

	if fencedInstances.Has(utils.FenceAllServers) {
		return true
	}
	return fencedInstances.Has(instance)
}

// ShouldCreateApplicationSecret returns true if for this cluster,
// during the bootstrap phase, we need to create a secret to store application credentials
func (cluster *Database) ShouldCreateApplicationSecret() bool {
	return cluster.ShouldInitDBCreateApplicationSecret() ||
		cluster.ShouldPgBaseBackupCreateApplicationSecret() ||
		cluster.ShouldRecoveryCreateApplicationSecret()
}

// ShouldInitDBCreateApplicationSecret returns true if for this cluster,
// during the bootstrap phase using initDB, we need to create an new application secret
func (cluster *Database) ShouldInitDBCreateApplicationSecret() bool {
	return cluster.ShouldInitDBCreateApplicationDatabase() &&
		(cluster.Spec.Bootstrap.InitDB.Secret == nil ||
			cluster.Spec.Bootstrap.InitDB.Secret.Name == "")
}

// ShouldPgBaseBackupCreateApplicationSecret returns true if for this cluster,
// during the bootstrap phase using pg_basebackup, we need to create an application secret
func (cluster *Database) ShouldPgBaseBackupCreateApplicationSecret() bool {
	return cluster.ShouldPgBaseBackupCreateApplicationDatabase() &&
		(cluster.Spec.Bootstrap.PgBaseBackup.Secret == nil ||
			cluster.Spec.Bootstrap.PgBaseBackup.Secret.Name == "")
}

// ShouldRecoveryCreateApplicationSecret returns true if for this cluster,
// during the bootstrap phase using recovery, we need to create an application secret
func (cluster *Database) ShouldRecoveryCreateApplicationSecret() bool {
	return cluster.ShouldRecoveryCreateApplicationDatabase() &&
		(cluster.Spec.Bootstrap.Recovery.Secret == nil ||
			cluster.Spec.Bootstrap.Recovery.Secret.Name == "")
}

// ShouldCreateApplicationDatabase returns true if for this cluster,
// during the bootstrap phase, we need to create an application database
func (cluster *Database) ShouldCreateApplicationDatabase() bool {
	return cluster.ShouldInitDBCreateApplicationDatabase() ||
		cluster.ShouldRecoveryCreateApplicationDatabase() ||
		cluster.ShouldPgBaseBackupCreateApplicationDatabase()
}

// ShouldInitDBRunPostInitApplicationSQLRefs returns true if for this cluster,
// during the bootstrap phase using initDB, we need to run post application
// SQL files from provided references.
func (cluster *Database) ShouldInitDBRunPostInitApplicationSQLRefs() bool {
	if cluster.Spec.Bootstrap == nil {
		return false
	}

	if cluster.Spec.Bootstrap.InitDB == nil {
		return false
	}

	if cluster.Spec.Bootstrap.InitDB.PostInitApplicationSQLRefs == nil {
		return false
	}

	return (len(cluster.Spec.Bootstrap.InitDB.PostInitApplicationSQLRefs.ConfigMapRefs) != 0 ||
		len(cluster.Spec.Bootstrap.InitDB.PostInitApplicationSQLRefs.SecretRefs) != 0)
}

// ShouldInitDBCreateApplicationDatabase returns true if the application database needs to be created during initdb
// job
func (cluster *Database) ShouldInitDBCreateApplicationDatabase() bool {
	if cluster.Spec.Bootstrap == nil {
		return false
	}

	if cluster.Spec.Bootstrap.InitDB == nil {
		return false
	}

	initDBParameters := cluster.Spec.Bootstrap.InitDB
	return initDBParameters.Owner != "" && initDBParameters.Database != ""
}

// ShouldPgBaseBackupCreateApplicationDatabase returns true if the application database needs to be created during the
// pg_basebackup job
func (cluster *Database) ShouldPgBaseBackupCreateApplicationDatabase() bool {
	// // we skip creating the application database if cluster is a replica
	// if cluster.IsReplica() {
	// 	return false
	// }
	if cluster.Spec.Bootstrap == nil {
		return false
	}

	if cluster.Spec.Bootstrap.PgBaseBackup == nil {
		return false
	}

	pgBaseBackupParameters := cluster.Spec.Bootstrap.PgBaseBackup
	return pgBaseBackupParameters.Owner != "" && pgBaseBackupParameters.Database != ""
}

// ShouldRecoveryCreateApplicationDatabase returns true if the application database needs to be created during the
// recovery job
func (cluster *Database) ShouldRecoveryCreateApplicationDatabase() bool {
	// // we skip creating the application database if cluster is a replica
	// if cluster.IsReplica() {
	// 	return false
	// }

	if cluster.Spec.Bootstrap == nil {
		return false
	}

	if cluster.Spec.Bootstrap.Recovery == nil {
		return false
	}

	recoveryParameters := cluster.Spec.Bootstrap.Recovery
	return recoveryParameters.Owner != "" && recoveryParameters.Database != ""
}

// ExternalCluster gets the external server with a known name, returning
// true if the server was found and false otherwise
func (cluster Database) ExternalCluster(name string) (ExternalCluster, bool) {
	for _, server := range cluster.Spec.ExternalClusters {
		if server.Name == name {
			return server, true
		}
	}

	return ExternalCluster{}, false
}

// // IsReplica checks if this is a replica cluster or not
// func (cluster Database) IsReplica() bool {
// 	return cluster.Spec.ReplicaCluster != nil && cluster.Spec.ReplicaCluster.Enabled
// }

// var slotNameNegativeRegex = regexp.MustCompile("[^a-z0-9_]+")

// UsesSecret checks whether a given secret is used by a Cluster.
//
// This function is also used to discover the set of clusters that
// should be reconciled when a certain secret changes.
func (cluster *Database) UsesSecret(secret string) bool {
	// if _, ok := cluster.Status.SecretsResourceVersion.Metrics[secret]; ok {
	// 	return true
	// }
	switch secret {
	case
		cluster.GetApplicationSecretName():
		return true
	}

	if cluster.UsesSecretInManagedRoles(secret) {
		return true
	}

	// if cluster.Status.PoolerIntegrations != nil {
	// 	for _, pgBouncerSecretName := range cluster.Status.PoolerIntegrations.PgBouncerIntegration.Secrets {
	// 		if pgBouncerSecretName == secret {
	// 			return true
	// 		}
	// 	}
	// }

	return false
}

// // UsesConfigMap checks whether a given secret is used by a Cluster
// func (cluster *Database) UsesConfigMap(config string) (ok bool) {
// 	if _, ok := cluster.Status.ConfigMapResourceVersion.Metrics[config]; ok {
// 		return true
// 	}
// 	return false
// }

// IsPodMonitorEnabled checks if the PodMonitor object needs to be created
func (cluster *Database) IsPodMonitorEnabled() bool {
	if cluster.Spec.Monitoring != nil {
		return cluster.Spec.Monitoring.EnablePodMonitor
	}

	return false
}

// SetInheritedDataAndOwnership sets the cluster as owner of the passed object and then
// sets all the needed annotations and labels
func (cluster *Database) SetInheritedDataAndOwnership(obj *metav1.ObjectMeta) {
	cluster.SetInheritedData(obj)
	utils.SetAsOwnedBy(obj, cluster.ObjectMeta, cluster.TypeMeta)
}

// SetInheritedData sets all the needed annotations and labels
func (cluster *Database) SetInheritedData(obj *metav1.ObjectMeta) {
	utils.InheritAnnotations(obj, cluster.Annotations, cluster.GetFixedInheritedAnnotations(), configuration.Current)
	utils.InheritLabels(obj, cluster.Labels, cluster.GetFixedInheritedLabels(), configuration.Current)
	utils.LabelClusterName(obj, cluster.GetName())
	utils.SetOperatorVersion(obj, versions.Version)
}

// ShouldForceLegacyBackup if present takes a backup without passing the name argument even on barman version 3.3.0+.
// This is needed to test both backup system in the E2E suite
func (cluster *Database) ShouldForceLegacyBackup() bool {
	return cluster.Annotations[utils.LegacyBackupAnnotationName] == "true"
}

// GetCoredumpFilter get the coredump filter value from the cluster annotation
func (cluster *Database) GetCoredumpFilter() string {
	value, ok := cluster.Annotations[utils.CoredumpFilter]
	if ok {
		return value
	}
	return system.DefaultCoredumpFilter
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}
