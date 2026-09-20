package scheduler

import (
	"reflect"
	"strings"
	"testing"

	predictorv1pb "veloxmesh/internal/scheduler/predictorv1"
	schedulerv1pb "veloxmesh/internal/scheduler/schedulerv1"

	"google.golang.org/protobuf/reflect/protoreflect"
)

var schedulerForbiddenFieldTerms = []string{
	"prompt",
	"message",
	"api_key",
	"authorization",
	"secret",
	"embedding",
	"payload",
	"raw",
	"semantic_cache",
}

func TestSchedulerPrivacyContractFieldNames(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"TaskFeature":            reflect.TypeOf(TaskFeature{}),
		"ScoreResult":            reflect.TypeOf(ScoreResult{}),
		"TrainingExportFeatures": reflect.TypeOf(TrainingExportFeatures{}),
		"TrainingExportLabels":   reflect.TypeOf(TrainingExportLabels{}),
	} {
		assertStructFieldsSafe(t, name, typ)
	}
	assertProtoFieldsSafe(t, "scheduler.v1.TaskFeature", (&schedulerv1pb.TaskFeature{}).ProtoReflect().Descriptor().Fields())
	assertProtoFieldsSafe(t, "predictor.v1.TaskFeature", (&predictorv1pb.TaskFeature{}).ProtoReflect().Descriptor().Fields())
}

func assertStructFieldsSafe(t *testing.T, owner string, typ reflect.Type) {
	t.Helper()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		assertSchedulerFieldNameSafe(t, owner, field.Name)
		if tag := strings.Split(field.Tag.Get("json"), ",")[0]; tag != "" && tag != "-" {
			assertSchedulerFieldNameSafe(t, owner, tag)
		}
	}
}

func assertProtoFieldsSafe(t *testing.T, owner string, fields protoreflect.FieldDescriptors) {
	t.Helper()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		assertSchedulerFieldNameSafe(t, owner, string(field.Name()))
		assertSchedulerFieldNameSafe(t, owner, field.JSONName())
	}
}

func assertSchedulerFieldNameSafe(t *testing.T, owner string, name string) {
	t.Helper()
	normalized := strings.ToLower(name)
	for _, term := range schedulerForbiddenFieldTerms {
		if strings.Contains(normalized, term) {
			t.Fatalf("%s exposes forbidden scheduler field term %q in %q", owner, term, name)
		}
	}
}
