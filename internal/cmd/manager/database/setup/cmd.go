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

// Package setupdb implements the "database setupdb" subcommand of the operator
package setup

import (
	"context"
	"os"

	"github.com/kballard/go-shellquote"
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudnative-pg/cloudnative-pg/internal/management/istio"
	"github.com/cloudnative-pg/cloudnative-pg/internal/management/linkerd"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/log"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres"
)

// NewCmd generates the "init" subcommand
func NewCmd() *cobra.Command {
	var appDBName string
	var appUser string
	var clusterName string
	var namespace string
	var postInitApplicationSQLStr string
	var postInitTemplateSQLStr string
	var postInitApplicationSQLRefsFolder string

	cmd := &cobra.Command{
		Use: "setupdb [options]",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return management.WaitKubernetesAPIServer(cmd.Context(), ctrl.ObjectKey{
				Name:      clusterName,
				Namespace: namespace,
			})
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			postInitApplicationSQL, err := shellquote.Split(postInitApplicationSQLStr)
			if err != nil {
				log.Error(err, "Error while parsing post init template SQL queries")
				return err
			}

			postInitTemplateSQL, err := shellquote.Split(postInitTemplateSQLStr)
			if err != nil {
				log.Error(err, "Error while parsing post init template SQL queries")
				return err
			}

			info := postgres.InitDbInfo{
				ApplicationDatabase:    appDBName,
				ApplicationUser:        appUser,
				ClusterName:            clusterName,
				Namespace:              namespace,
				PostInitApplicationSQL: postInitApplicationSQL,
				PostInitTemplateSQL:    postInitTemplateSQL,
				// if the value to postInitApplicationSQLRefsFolder is empty,
				// bootstrap will do nothing for post init application SQL refs.
				PostInitApplicationSQLRefsFolder: postInitApplicationSQLRefsFolder,
			}

			return initSubCommand(ctx, info)
		},
		PostRunE: func(cmd *cobra.Command, args []string) error {
			if err := istio.TryInvokeQuitEndpoint(cmd.Context()); err != nil {
				return err
			}

			return linkerd.TryInvokeShutdownEndpoint(cmd.Context())
		},
	}

	cmd.Flags().StringVar(&appDBName, "app-db-name", "app",
		"The name of the application containing the database")
	cmd.Flags().StringVar(&appUser, "app-user", "app",
		"The name of the application user")
	cmd.Flags().StringVar(&clusterName, "cluster-name", os.Getenv("CLUSTER_NAME"), "The name of the "+
		"current cluster in k8s, used to coordinate switchover and failover")
	cmd.Flags().StringVar(&namespace, "namespace", os.Getenv("NAMESPACE"), "The namespace of "+
		"the cluster and the pod in k8s")
	cmd.Flags().StringVar(&postInitApplicationSQLStr, "post-init-application-sql", "", "The list of SQL queries to be "+
		"executed inside application database right after the database is created")
	cmd.Flags().StringVar(&postInitTemplateSQLStr, "post-init-template-sql", "", "The list of SQL queries to be "+
		"executed inside template1 database to configure the new instance")
	cmd.Flags().StringVar(&postInitApplicationSQLRefsFolder, "post-init-application-sql-refs-folder",
		"", "The folder contains a set of SQL files to be executed in alphabetical order "+
			"against the application database immediately after its creationd")

	return cmd
}

func initSubCommand(ctx context.Context, info postgres.InitDbInfo) error {
	err := info.Bootstrap(ctx)
	if err != nil {
		log.Error(err, "Error while bootstrapping database")
		return err
	}

	return nil
}
