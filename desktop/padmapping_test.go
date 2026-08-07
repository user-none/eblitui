package desktop

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

const testGUID = "030000007e0500000920000000020000"

func TestGenerateBaselineMappingSticksAndTriggers(t *testing.T) {
	// Pro-Controller-like: 12 buttons, 4 stick axes resting at 0 and two
	// trigger axes resting at -1.
	line := GenerateBaselineMapping(testGUID, "Pro Controller", 12, []float64{0, 0, 0, 0, -1, -1})

	if !strings.HasPrefix(line, testGUID+",Pro Controller,") {
		t.Fatalf("line prefix wrong: %s", line)
	}

	for _, want := range []string{
		"leftx:a0", "lefty:a1", "rightx:a2", "righty:a3",
		"lefttrigger:a4", "righttrigger:a5",
		"a:b0", "b:b1", "x:b2", "y:b3",
		"leftshoulder:b4", "rightshoulder:b5",
		"back:b6", "start:b7", "leftstick:b8", "rightstick:b9",
		"guide:b10",
		"dpup:h0.1", "dpright:h0.2", "dpdown:h0.4", "dpleft:h0.8",
	} {
		if !strings.Contains(line, ","+want+",") {
			t.Errorf("missing element %q in %s", want, line)
		}
	}
	if strings.Contains(line, ":b11") {
		t.Errorf("button beyond slot count assigned in %s", line)
	}

	// Trigger slots are taken by axes, so no button may be assigned to them.
	for _, bad := range []string{"lefttrigger:b", "righttrigger:b"} {
		if strings.Contains(line, bad) {
			t.Errorf("unexpected button element %q in %s", bad, line)
		}
	}
}

func TestGenerateBaselineMappingButtonsOnly(t *testing.T) {
	line := GenerateBaselineMapping(testGUID, "Pad", 4, nil)

	for _, want := range []string{"a:b0", "b:b1", "x:b2", "y:b3", "dpup:h0.1"} {
		if !strings.Contains(line, ","+want+",") {
			t.Errorf("missing element %q in %s", want, line)
		}
	}
	for _, bad := range []string{"leftx:", "lefty:", "lefttrigger:", "leftshoulder:"} {
		if strings.Contains(line, bad) {
			t.Errorf("unexpected element %q in %s", bad, line)
		}
	}
}

func TestGenerateBaselineMappingSlotLimits(t *testing.T) {
	// More buttons than assignable slots: only the 13 slots are used.
	line := GenerateBaselineMapping(testGUID, "Pad", 20, nil)
	if !strings.Contains(line, ",righttrigger:b11,") || !strings.Contains(line, ",guide:b12,") {
		t.Errorf("expected slots through guide:b12 in %s", line)
	}
	if strings.Contains(line, ":b13") {
		t.Errorf("button beyond slot count assigned in %s", line)
	}

	// More stick and trigger axes than slots: extras are ignored.
	line = GenerateBaselineMapping(testGUID, "Pad", 0, []float64{0, 0, 0, 0, 0, -1, -1, -1})
	if strings.Contains(line, ":a4,") {
		t.Errorf("fifth stick axis assigned in %s", line)
	}
	if strings.Contains(line, ":a7,") {
		t.Errorf("third trigger axis assigned in %s", line)
	}
	if !strings.Contains(line, ",lefttrigger:a5,") || !strings.Contains(line, ",righttrigger:a6,") {
		t.Errorf("trigger axes not assigned in %s", line)
	}
}

func TestGenerateBaselineMappingTriggerBeforeSticks(t *testing.T) {
	// Axis order does not decide classification; resting value does.
	line := GenerateBaselineMapping(testGUID, "Pad", 0, []float64{-1, 0, 0.02})
	for _, want := range []string{"lefttrigger:a0", "leftx:a1", "lefty:a2"} {
		if !strings.Contains(line, ","+want+",") {
			t.Errorf("missing element %q in %s", want, line)
		}
	}
	if strings.Contains(line, "righttrigger:") {
		t.Errorf("unexpected righttrigger in %s", line)
	}
}

func TestGenerateBaselineMappingNameAndGUID(t *testing.T) {
	if got := GenerateBaselineMapping("", "Pad", 4, nil); got != "" {
		t.Errorf("empty GUID: got %q, want empty", got)
	}

	line := GenerateBaselineMapping(testGUID, "Weird,Name:Test", 1, nil)
	if !strings.HasPrefix(line, testGUID+",WeirdNameTest,") {
		t.Errorf("name not sanitized: %s", line)
	}

	line = GenerateBaselineMapping(testGUID, "  ", 1, nil)
	if !strings.HasPrefix(line, testGUID+",Controller,") {
		t.Errorf("empty name not defaulted: %s", line)
	}
}

func TestGenerateBaselineMappingSaturnStylePad(t *testing.T) {
	// Saturn-style pad through the macOS GameController backend: 12 raw
	// buttons (b9 and b11 enumerated but never firing), triggers on a2/a5
	// interleaved between the stick axes.
	line := GenerateBaselineMapping(testGUID, "Pro Controller", 12, []float64{0, 0, -1, 0, 0, -1})

	for _, want := range []string{
		"leftx:a0", "lefty:a1", "rightx:a3", "righty:a4",
		"lefttrigger:a2", "righttrigger:a5",
		"a:b0", "b:b1", "x:b2", "y:b3",
		"leftshoulder:b4", "rightshoulder:b5",
		"back:b6", "start:b7", "leftstick:b8", "rightstick:b9",
		"guide:b10",
	} {
		if !strings.Contains(line, ","+want+",") {
			t.Errorf("missing element %q in %s", want, line)
		}
	}
	if strings.Contains(line, ":b11") {
		t.Errorf("button beyond slot count assigned in %s", line)
	}
}

func TestGenerateBaselineMappingApplies(t *testing.T) {
	// The generated line must parse in the gamepad mapping database.
	line := GenerateBaselineMapping(testGUID, "Pro Controller", 12, []float64{0, 0, 0, 0, -1, -1})
	applied, err := ebiten.UpdateStandardGamepadLayoutMappings(line)
	if err != nil {
		t.Fatalf("generated mapping failed to apply: %v", err)
	}
	if !applied {
		t.Fatal("generated mapping was not applied")
	}
}
