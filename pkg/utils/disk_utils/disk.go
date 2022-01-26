package disk_utils

import (
	"context"
	"fmt"
	"github.com/dashjay/bazel-remote-exec/pkg/utils"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"path/filepath"
)

func WriteFile(ctx context.Context, fullPath string, data []byte) (int64, error) {
	randStr := utils.RandomString(10)

	tmpFileName := fmt.Sprintf("%s.%s.tmp", fullPath, randStr)
	err := os.MkdirAll(filepath.Dir(fullPath), 0644)
	if err != nil {
		return 0, err
	}

	defer DeleteLocalFileIfExists(tmpFileName)

	if err := ioutil.WriteFile(tmpFileName, data, 0644); err != nil {
		return 0, err
	}
	return int64(len(data)), os.Rename(tmpFileName, fullPath)
}


func DeleteLocalFileIfExists(filename string) {
	_, err := os.Stat(filename)
	if err == nil {
		if err := os.Remove(filename); err != nil {
			logrus.Warningf("Error deleting file %q: %s", filename, err)
		}
	}
}
