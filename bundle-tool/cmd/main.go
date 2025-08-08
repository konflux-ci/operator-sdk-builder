package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "bundle-tool",
	Short: "A tool for working with OLM bundles and creating Konflux snapshots",
	Long: `bundle-tool is a comprehensive toolkit for OLM bundle operations including:
- Creating Konflux snapshots from bundle images
- Manipulating bundle image references
- Generating relatedImages sections
- Converting image references between registries`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
