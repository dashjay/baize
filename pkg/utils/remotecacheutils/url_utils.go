package remotecacheutils

import (
	"fmt"
	"strings"
)

type CacheAction struct {
	CacheType    string
	Digest       string
	InstanceName string
}

func (c *CacheAction) String() string {
	if c.InstanceName == "" {
		c.InstanceName = "default"
	}
	return fmt.Sprintf("/%s/%s/%s", c.InstanceName, c.CacheType, c.Digest)
}

// Parse url to CacheAction
// /{instance_name?}/{ac,cas}/{digest}
func Parse(url string) *CacheAction {
	var ca CacheAction
	ca.InstanceName = "default"
	if strings.Count(url, "/") == 3 {
		url = url[1:]
		slash := strings.Index(url, "/")
		ca.InstanceName = url[:slash]
		url = url[slash:]
	}
	url = url[1:]
	slash := strings.Index(url, "/")
	ca.CacheType = url[:slash]
	url = url[slash+1:]
	ca.Digest = url
	return &ca
}
