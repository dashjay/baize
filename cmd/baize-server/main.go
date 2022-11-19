package main

import (
	"github.com/spf13/cobra"
	"k8s.io/kubernetes/pkg/util/rlimit"

	_ "net/http/pprof"

	"github.com/dashjay/baize/pkg/baize"
	"github.com/dashjay/baize/pkg/cc"
)

func init() {
	err := rlimit.SetNumFiles(4096)
	if err != nil {
		panic(err)
	}
}
func NewBazelExecutorCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cfgPath := cmd.Flags().String("config", "/config.toml", "config file to use")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		cfg, err := cc.NewConfigFromFile(*cfgPath)
		if err != nil {
			return err
		}
		s, err := baize.New(cfg)
		if err != nil {
			return err
		}
		return s.Run()
	}
	return cmd
}

func main() {
	err := NewBazelExecutorCommand().Execute()
	if err != nil {
		panic(err)
	}
}
