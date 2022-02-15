package main

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dashjay/baize/pkg/utils/healthchecker"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/kubernetes/pkg/util/rlimit"

	"github.com/dashjay/baize/pkg/caches"
	"github.com/dashjay/baize/pkg/config"
	"github.com/dashjay/baize/pkg/interfaces"
	rc "github.com/dashjay/baize/pkg/utils/remotecacheutils"
)

func init() {
	err := rlimit.SetNumFiles(4096)
	if err != nil {
		panic(err)
	}
}

const (
	ac  = "ac"
	cas = "cas"
)

func readFromCache(w http.ResponseWriter, r *http.Request, d *repb.Digest, cache interfaces.Cache) {
	w.Header().Set("Content-Encoding", "gzip")
	c, err := cache.Reader(r.Context(), d, 0)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer c.Close()
	_, err = io.Copy(w, c)
	if err != nil {
		cache.Delete(r.Context(), d)
		_, _ = w.Write([]byte(fmt.Sprintf("copy body error: %s", err)))
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func read(w http.ResponseWriter, r *http.Request, cache interfaces.Cache) {
	cacheAction := rc.Parse(r.RequestURI)
	if cacheAction.CacheType == cas {
		isoCache, _ := cache.WithIsolation(r.Context(), interfaces.CASCacheType, cacheAction.InstanceName)
		readFromCache(w, r, &repb.Digest{Hash: cacheAction.Digest}, isoCache)
	}
	if cacheAction.CacheType == ac {
		acCache, _ := cache.WithIsolation(r.Context(), interfaces.ActionCacheType, cacheAction.InstanceName)
		readFromCache(w, r, &repb.Digest{Hash: cacheAction.Digest}, acCache)
	}
}

func writeToCache(w http.ResponseWriter, r *http.Request, d *repb.Digest, cache interfaces.Cache) {
	var err error
	defer func() {
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("writeToCache error: %s", err)))
			cache.Delete(r.Context(), d)
		}
	}()
	wc, err := cache.Writer(r.Context(), d)
	if err != nil {
		return
	}
	gw := gzip.NewWriter(wc)
	_, err = io.Copy(gw, r.Body)
	if err != nil {
		return
	}
	err = gw.Close()
	if err != nil {
		return
	}
	err = wc.Close()
	if err != nil {
		return
	}
}

func write(w http.ResponseWriter, r *http.Request, cache interfaces.Cache) {
	cacheAction := rc.Parse(r.RequestURI)
	if cacheAction.CacheType == cas {
		isoCache, _ := cache.WithIsolation(r.Context(), interfaces.CASCacheType, cacheAction.InstanceName)
		writeToCache(w, r, &repb.Digest{Hash: cacheAction.Digest}, isoCache)
	}
	if cacheAction.CacheType == ac {
		acCache, _ := cache.WithIsolation(r.Context(), interfaces.ActionCacheType, cacheAction.InstanceName)
		writeToCache(w, r, &repb.Digest{Hash: cacheAction.Digest}, acCache)
	}
}

type Handler struct {
	cache interfaces.Cache
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		read(w, r, h.cache)
	}
	if r.Method == http.MethodPut {
		write(w, r, h.cache)
	}
}

func NewRemoteCacheCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cfgPath := cmd.Flags().String("config", "/config.toml", "config file to use")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		cfg, err := config.NewConfigFromFile(*cfgPath)
		if err != nil {
			return err
		}
		l, err := logrus.ParseLevel(cfg.GetDebugConfig().LogLevel)
		if err != nil {
			return err
		}
		logrus.SetLevel(l)
		cacheCfg := cfg.GetCacheConfig()
		cache := caches.GenerateCacheFromConfig(cacheCfg)
		if cache == nil {
			return errors.New("no cache enabled")
		}
		hc := healthchecker.NewHealthchecker()
		hc.AddChecker(cache.Check, time.Second*60)
		hc.Start()
		return http.ListenAndServe(cacheCfg.ListenAddr, &Handler{cache: cache})
	}
	return cmd
}

func main() {
	err := NewRemoteCacheCommand().Execute()
	if err != nil {
		panic(err)
	}
}
