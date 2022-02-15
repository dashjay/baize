package main

import (
	"net/http"

	"github.com/spf13/cobra"
	"k8s.io/kubernetes/pkg/util/rlimit"

	_ "net/http/pprof"

	"github.com/dashjay/baize/pkg/baize"
	"github.com/dashjay/baize/pkg/config"
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
		cfg, err := config.NewConfigFromFile(*cfgPath)
		if err != nil {
			return err
		}
		s, err := baize.New(cfg)
		if err != nil {
			return err
		}
		if pprofAddr := cfg.GetExecutorConfig().PprofAddr; pprofAddr != "" {
			go func() {
				http.ListenAndServe(pprofAddr, nil)
			}()
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
