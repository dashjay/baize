package server

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/sirupsen/logrus"
)

func marshalAny(pb proto.Message) (*any.Any, error) {
	pbAny, err := ptypes.MarshalAny(pb)
	if err != nil {
		s := fmt.Sprintf("Failed to marshal proto message %q as Any: %s", pb, err)
		logrus.WithError(err).Error(s)
		return nil, err
	}
	return pbAny, nil
}

func IsValidDigest(hash string, size int64) bool {
	// TODO: use inner functions to check sha256 hex size
	return len(hash) == 64 && size >= -1
}
