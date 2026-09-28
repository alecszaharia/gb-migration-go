/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package global_blocks

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCommand(groupName string) *cobra.Command {
	var migrationCmd = &cobra.Command{
		GroupID: groupName,
		Use:     "global_blocks",
		Short:   "Migrate Global Blocks from the old database structure to the new one",
		Long: `Migrate Brizy Global Blocks from the legacy database structure to the new
structure that is served through the GraphQL API.

The command reads Global Blocks stored in the old schema, transforms them into
the new data model, and writes them to the new tables so they can be queried
through the GraphQL API.`,
		Aliases: []string{"gb"},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if viper.GetString("database_url") == "" {
				return fmt.Errorf("please provide the [-d, --database_url string] flag or set the  DATABASE_URL env var.")
			}
			return nil
		},
		RunE: migrationRun,
	}

	migrationCmd.Flags().StringVarP(&database_url, "database_url", "d", "", "Database connection string")
	migrationCmd.Flags().IntVarP(&batch, "batch", "b", 100, "Batch count")
	migrationCmd.Flags().BoolVarP(&failed, "failed", "f", false, "Iterate through failed blocks only")

	// Bind flag to Viper key
	_ = viper.BindPFlag("database_url", migrationCmd.Flags().Lookup("database_url"))
	_ = viper.BindPFlag("batch", migrationCmd.Flags().Lookup("batch"))
	_ = viper.BindPFlag("failed", migrationCmd.Flags().Lookup("failed"))
	viper.AutomaticEnv()

	return migrationCmd
}
