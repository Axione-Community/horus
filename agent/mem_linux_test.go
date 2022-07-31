package agent

import "testing"

func TestTotalMem(t *testing.T) {
	totalMem := getTotalMem()
	if totalMem == 0 {
		t.Fatalf("invalid total mem")
	}
	t.Logf("total mem: %.0fb / %.0fkB", totalMem, totalMem/1024)
	usedMem := getUsedMem()
	if usedMem == 0 {
		t.Fatalf("invalid used mem")
	}
	t.Logf("total used: %.0fb / %.0fkB", usedMem, usedMem/1024)
}
