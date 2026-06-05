package romloader

import "testing"

func TestSynthIndex01(t *testing.T) {
	n, lba := synthIndex01(1000, 150)
	if n != 1 || lba != 1150 {
		t.Errorf("synthIndex01(1000,150) = {%d %d}, want {1 1150}", n, lba)
	}
	n, lba = synthIndex01(0, 0)
	if n != 1 || lba != 0 {
		t.Errorf("synthIndex01(0,0) = {%d %d}, want {1 0}", n, lba)
	}
}
