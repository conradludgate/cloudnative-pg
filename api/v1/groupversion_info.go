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

// Package v1 contains API Schema definitions for the postgresql v1 API group
// +kubebuilder:object:generate=true
// +groupName=postgresql.cnpg.io
package v1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "postgresql.cnpg.io", Version: "v1"}

	// ClusterGVK is the triple to reach Cluster resources in k8s
	ClusterGVK = schema.GroupVersionResource{
		Group:    GroupVersion.Group,
		Version:  GroupVersion.Version,
		Resource: "clusters",
	}

	// PoolerGVK is the triple to reach Pooler resources in k8s
	PoolerGVK = schema.GroupVersionResource{
		Group:    GroupVersion.Group,
		Version:  GroupVersion.Version,
		Resource: "poolers",
	}

	// ClusterKind is the kind name of Clusters
	ClusterKind = "Cluster"

	// DatabaseKind is the kind name of Databases
	DatabaseKind = "Database"

	// BackupKind is the kind name of Backups
	BackupKind = "Backup"

	// PoolerKind is the kind name of Poolers
	PoolerKind = "Pooler"

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
