package input

import "testing"

func TestDoctorAdmissionUsesPrivateCheckpoint(t *testing.T) {
	for _, raw := range []string{`{"payload":{"operator_operation":"doctor.check_system"}}`, `{"operator_operation":"http.request"}`, `invalid`} {
		if doctorAdmission(raw) {
			t.Fatal("untrusted input selected operator path")
		}
	}
	if !doctorAdmission(`{"operator_operation":"doctor.check_system","prepared":{}}`) {
		t.Fatal("trusted marker lost during checkpoint")
	}
}
