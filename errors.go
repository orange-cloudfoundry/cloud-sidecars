package sidecars

import (
	"fmt"
	"github.com/orange-cloudfoundry/cloud-sidecars/config"
)

type sidecarError struct {
	s   *config.Sidecar
	err error
}

func NewSidecarError(s *config.Sidecar, err error) *sidecarError {
	return &sidecarError{s, err}
}

func (e sidecarError) Error() string {
	return fmt.Sprintf("Error on sidecar %s: %s", e.s.Name, e.err.Error())
}
