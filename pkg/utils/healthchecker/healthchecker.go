package healthchecker

import (
	"context"
	"reflect"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
)

type CheckFunc func(ctx context.Context) error

type HealthChecker struct {
	checkers map[time.Duration][]CheckFunc
}

func NewHealthchecker() *HealthChecker {
	return &HealthChecker{checkers: make(map[time.Duration][]CheckFunc)}
}

func (h *HealthChecker) AddChecker(checker CheckFunc, interval time.Duration) {
	if checker == nil {
		panic("invalid checker")
	}
	if _, exists := h.checkers[interval]; exists {
		h.checkers[interval] = append(h.checkers[interval], checker)
	} else {
		h.checkers[interval] = make([]CheckFunc, 1)
		h.checkers[interval][0] = checker
	}
}

func getFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}
func (h *HealthChecker) Start() {
	if len(h.checkers) == 0 {
		return
	}
	ctx := context.Background()
	receive := make(chan error)
	for interval := range h.checkers {
		for idx := range h.checkers[interval] {
			go func(fn CheckFunc) {
				logrus.Infof("register checker %s for every %d", getFunctionName(fn), interval)
				tk := time.NewTicker(interval)
				for range tk.C {
					err := fn(ctx)
					if err != nil {
						receive <- err
					}
				}
			}(h.checkers[interval][idx])
		}
	}

	go func() {
		for err := range receive {
			if err != nil {
				logrus.WithError(err).Errorf("[Healthchcker] error received")
			}
		}
	}()
}
