package main

import (
	"github.com/dashjay/bazel-remote-exec/pkg/config"
	"github.com/dashjay/bazel-remote-exec/pkg/server"
	"github.com/spf13/cobra"
)

func NewExecutorCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cfgPath := cmd.Flags().String("config", "/config.yaml", "config file to use")
	cmd.Run = func(cmd *cobra.Command, args []string) {
		cfg, err := config.NewConfigFromFile(*cfgPath)
		if err != nil {
			panic(err)
		}
		s, err := server.New(cfg)
		if err != nil {
			panic(err)
		}
		panic(s.Run())
	}
	return cmd
}

func main() {
	err := NewExecutorCommand().Execute()
	if err != nil {
		panic(err)
	}
}
