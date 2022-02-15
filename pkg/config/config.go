package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/sirupsen/logrus"
)

type DebugConfig struct {
	LogLevel string `toml:"log_level"`
}
type ServerConfig struct {
	ListenAddr string `toml:"listen_addr"`
	PprofAddr  string `toml:"pprof_addr"`
}

type ExecutorConfig struct {
	ListenAddr string `toml:"listen_addr"`
	PprofAddr  string `toml:"pprof_addr"`
	WorkDir    string `toml:"work_dir"`
}

type CacheConfig struct {
	ListenAddr    string `toml:"listen_addr"`
	RedisCache    *Cache `toml:"redis_cache"`
	DiskCache     *Cache `toml:"disk_cache"`
	InmemoryCache *Cache `toml:"inmemory_cache"`
}

// Cache config for every config type
type Cache struct {
	// Enabled if this cache enabled
	Enabled bool `toml:"enabled"`

	// CacheAddr is an addr to cache
	// - A network address to redis or other
	// - A path to position on disk
	CacheAddr string `toml:"cache_addr"`

	// CacheSize is `max` size the cache can take in.
	// Because we limit caches size by lru maybe the actual usage of size will surpass the CacheSize
	CacheSize int64 `toml:"cache_size"`

	// UnitSizeLimitation is max unit size cache can take in
	UnitSizeLimitation int `toml:"unit_size_limitation"`
}

func (c *Cache) String() string {
	return fmt.Sprintf("%#v", *c)
}

type Configure struct {
	ExecutorConfig `toml:"executor"`
	ServerConfig   `toml:"server"`
	DebugConfig    `toml:"debug"`
	CacheConfig    `toml:"caches"`
}

func (c *CacheConfig) String() string {
	return fmt.Sprintf("DiskCache: %#v\nInmemoryCache: %#v\nRedisCache: %#v\n", c.DiskCache, c.InmemoryCache, c.RedisCache)
}

func (c *Configure) String() string {
	return fmt.Sprintf("%#v\n%#v\n%s\n", c.DebugConfig, c.ExecutorConfig, c.CacheConfig.String())
}

func (c *Configure) GetCacheConfig() *CacheConfig {
	return &c.CacheConfig
}

func (c *Configure) GetExecutorConfig() *ExecutorConfig {
	return &c.ExecutorConfig
}

func (c *Configure) GetServerConfig() *ServerConfig {
	return &c.ServerConfig
}

func (c *Configure) GetDebugConfig() *DebugConfig {
	return &c.DebugConfig
}

func NewConfigFromFile(configFilePath string) (*Configure, error) {
	var cfg Configure
	_, err := toml.DecodeFile(configFilePath, &cfg)
	if err != nil {
		return nil, err
	}
	logrus.Infof("unmarshaled:\n%s\n", cfg.String())
	return &cfg, nil
}
