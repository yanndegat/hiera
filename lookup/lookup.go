package main

import (
	"fmt"
	"os"

	"github.com/yanndegat/hiera/cli"
)

func main() {
	cmd := cli.NewCommand()
	err := cmd.Execute()
	if err != nil {
		fmt.Println(cmd.OutOrStderr(), err)
		os.Exit(1)
	}
}
