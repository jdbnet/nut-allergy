package snmp

import "testing"

func TestInterpret(t *testing.T) {
	tests := []struct {
		src, bat int
		want     string
	}{
		{3, 2, StateOnline},
		{4, 2, StateOnline},
		{6, 3, StateOnline},
		{5, 2, StateOnBattery},
		{5, 3, StateLowBattery},
		{5, 4, StateLowBattery},
		{2, 2, StateLowBattery},
		{1, 1, StateUnknown},
		{0, 0, StateUnknown},
	}
	for _, tt := range tests {
		if got := Interpret(tt.src, tt.bat); got != tt.want {
			t.Fatalf("Interpret(%d,%d)=%s want %s", tt.src, tt.bat, got, tt.want)
		}
	}
}

func TestValidateTarget(t *testing.T) {
	ok := Target{Host: "10.0.0.1", User: "nut", SecurityLevel: "authNoPriv", AuthProtocol: "SHA", AuthPassword: "secretsecret"}
	if err := ValidateTarget(ok); err != nil {
		t.Fatal(err)
	}
	ok.SecurityLevel = "authPriv"
	if err := ValidateTarget(ok); err == nil {
		t.Fatal("expected privacy password error")
	}
	ok.PrivProtocol = "AES"
	ok.PrivPassword = "secretsecret"
	if err := ValidateTarget(ok); err != nil {
		t.Fatal(err)
	}
}
