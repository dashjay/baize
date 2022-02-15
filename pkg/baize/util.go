package baize

import (
	"fmt"

	"github.com/golang/protobuf/ptypes/any"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func marshalAny(pb proto.Message) (*any.Any, error) {
	pbAny, err := anypb.New(pb)
	if err != nil {
		logrus.WithError(err).Error(fmt.Sprintf("Failed to marshal proto message %q as Any: %s", pb, err))
		return nil, err
	}
	return pbAny, nil
}

func IsValidDigest(hash string, size int64) bool {
	// TODO: use inner functions to check sha256 hex size
	return len(hash) == 64 && size >= -1
}
