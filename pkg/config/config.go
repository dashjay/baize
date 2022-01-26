package config

import (
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"os"
)

type DebugConfig struct {
	LogLevel string `yaml:"log_level"`
}

type ExecutorConfig struct {
	ListenAddr string `yaml:"listen_addr"`
	WorkDir    string `yaml:"work_dir"`
}

type CacheConfig struct {
	RedisCache    *Cache `yaml:"redis_cache"`
	DiskCache     *Cache `yaml:"disk_cache"`
	InmemoryCache *Cache `yaml:"inmemory_cache"`
}

// Cache config for every config type
type Cache struct {
	// Enabled if this cache enabled
	Enabled bool `yaml:"enabled"`

	// CacheAddr is an addr to cache
	// - A network address to redis or other
	// - A path to position on disk
	CacheAddr string `yaml:"cache_addr"`

	// CacheSize is `max` size the cache can use.
	// Because we limit caches size by lru maybe the actual usage of size will surpass the CacheSize
	CacheSize int64 `yaml:"cache_size"`
}

type Configure struct {
	DebugConfig    `yaml:"debug"`
	ExecutorConfig `yaml:"executor"`
	CacheConfig    `yaml:"caches"`
}

func (c *Configure) GetCacheConfig() *CacheConfig {
	return &c.CacheConfig
}

func (c *Configure) GetExecutorConfig() *ExecutorConfig {
	return &c.ExecutorConfig
}

func (c *Configure) GetDebugConfig() *DebugConfig {
	return &c.DebugConfig
}

func NewConfigFromFile(configFilePath string) (*Configure, error) {
	var cfg Configure
	content, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, err
	}
	logrus.Infof("read config file:\n%s\n", content)
	err = yaml.Unmarshal(content, &cfg)
	if err != nil {
		return nil, err
	}
	logrus.Infof("unmarshaled:\n%#v\n", cfg)
	return &cfg, nil
}
