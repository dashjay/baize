package main

import (
	"github.com/spf13/cobra"
	"k8s.io/kubernetes/pkg/util/rlimit"

	"github.com/dashjay/baize/pkg/cc"
)

func init() {
	err := rlimit.SetNumFiles(4096)
	if err != nil {
		panic(err)
	}
}
func NewBazelServerCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cfgPath := cmd.Flags().String("config", "/config.toml", "config file to use")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		_, err := cc.NewConfigFromFile(*cfgPath)
		if err != nil {
			return err
		}
		return nil
	}
	return cmd
}

func main() {
	err := NewBazelServerCommand().Execute()
	if err != nil {
		panic(err)
	}
}
