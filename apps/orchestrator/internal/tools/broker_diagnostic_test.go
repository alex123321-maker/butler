package tools

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBrokerActionErrorKeepsOnlyFixedRPCClass(t *testing.T) {
	for _, tc := range []struct {
		code codes.Code
		want string
	}{
		{codes.PermissionDenied, "broker_permission_denied"}, {codes.Unauthenticated, "broker_unauthenticated"},
		{codes.InvalidArgument, "broker_invalid_argument"}, {codes.FailedPrecondition, "broker_failed_precondition"},
		{codes.DeadlineExceeded, "broker_deadline_exceeded"}, {codes.Unavailable, "broker_unavailable"},
		{codes.Canceled, "broker_cancelled"}, {codes.Internal, "broker_transport"},
	} {
		cause := status.Error(tc.code, "PRIVATE_ARG SECRET credential")
		got := brokerActionError(cause)
		if got.DiagnosticCode() != tc.want || !errors.Is(got, cause) || strings.Contains(got.Error(), "PRIVATE") || strings.Contains(got.Error(), "SECRET") {
			t.Fatal("unsafe or incorrect RPC classification")
		}
	}
	if brokerActionError(errors.New("PRIVATE")).Code != "broker_transport" {
		t.Fatal("unknown error guessed")
	}
}
