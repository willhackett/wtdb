// Command wtdb gives each git worktree its own isolated database.
package main

import (
	"os"

	"github.com/willhackett/wtdb/internal/cli"

	// Register the bundled database adapters.
	_ "github.com/willhackett/wtdb/internal/db/postgres"
)

func main() {
	os.Exit(cli.Main(os.Args))
}
