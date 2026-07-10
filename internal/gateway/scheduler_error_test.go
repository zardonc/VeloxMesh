package gateway

import (
	"testing"

	"veloxmesh/internal/errors"
	"veloxmesh/internal/scheduler"
)

func TestSchedulerGatewayErrorMapsDuplicateTaskToConflict(t *testing.T) {
	err := schedulerGatewayError(scheduler.ErrDuplicateTask)
	gwErr, ok := err.(*errors.GatewayError)
	if !ok {
		t.Fatalf("expected gateway error, got %T", err)
	}
	if gwErr.Code != errors.SchedulerDuplicateTask || gwErr.HTTPStatus != 409 {
		t.Fatalf("expected duplicate task conflict, got %s/%d", gwErr.Code, gwErr.HTTPStatus)
	}
}
