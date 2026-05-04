package settings

import (
	"errors"
	"testing"
)

func TestCatalog_EveryKnobHasDescription(t *testing.T) {
	for _, k := range Catalog() {
		if k.Description == "" {
			t.Errorf("knob %q has empty Description", k.Key)
		}
	}
}

func TestCatalog_AllSliderKnobsHaveMinMaxStep(t *testing.T) {
	for _, k := range Catalog() {
		if k.Type != KnobInt && k.Type != KnobFloat {
			continue
		}
		if k.SliderMin == nil || k.SliderMax == nil || k.SliderStep == nil {
			t.Errorf("numeric knob %q missing slider fields: min=%v max=%v step=%v",
				k.Key, k.SliderMin, k.SliderMax, k.SliderStep)
			continue
		}
		if *k.SliderMax <= *k.SliderMin {
			t.Errorf("knob %q: slider_max (%v) must be greater than slider_min (%v)",
				k.Key, *k.SliderMax, *k.SliderMin)
		}
		if *k.SliderStep <= 0 {
			t.Errorf("knob %q: slider_step (%v) must be positive", k.Key, *k.SliderStep)
		}
	}
}

func TestCatalog_AllEnumKnobsHaveEnumValues(t *testing.T) {
	for _, k := range Catalog() {
		if k.Type != KnobEnum {
			continue
		}
		if len(k.EnumValues) == 0 {
			t.Errorf("enum knob %q has no EnumValues", k.Key)
		}
	}
}

func TestKnobByKey_ReturnsEntry(t *testing.T) {
	got, ok := KnobByKey("max_parallel")
	if !ok {
		t.Fatalf("KnobByKey(%q) returned ok=false", "max_parallel")
	}
	if got.Key != "max_parallel" {
		t.Errorf("KnobByKey returned key %q, want %q", got.Key, "max_parallel")
	}
	if got.Type != KnobInt {
		t.Errorf("KnobByKey returned type %q, want %q", got.Type, KnobInt)
	}
}

func TestKnobByKey_ReturnsFalseForUnknown(t *testing.T) {
	if _, ok := KnobByKey("does_not_exist"); ok {
		t.Fatalf("KnobByKey(%q) returned ok=true, want false", "does_not_exist")
	}
}

func TestSystemDefault_EveryKnobHasDefault(t *testing.T) {
	for _, k := range Catalog() {
		got, ok := SystemDefault(k.Key)
		if !ok {
			t.Errorf("SystemDefault(%q) returned ok=false", k.Key)
			continue
		}
		if got == nil && k.Type != KnobString {
			t.Errorf("SystemDefault(%q) returned nil", k.Key)
		}
	}
}

func TestSystemDefault_TypesMatchKnobType(t *testing.T) {
	for _, k := range Catalog() {
		def, ok := SystemDefault(k.Key)
		if !ok {
			t.Fatalf("SystemDefault(%q) missing", k.Key)
		}
		switch k.Type {
		case KnobInt:
			if _, ok := def.(int); !ok {
				t.Errorf("knob %q: int default is %T, want int", k.Key, def)
			}
		case KnobFloat:
			if _, ok := def.(float64); !ok {
				t.Errorf("knob %q: float default is %T, want float64", k.Key, def)
			}
		case KnobString, KnobEnum:
			if _, ok := def.(string); !ok {
				t.Errorf("knob %q: %s default is %T, want string", k.Key, k.Type, def)
			}
		case KnobMultiselect:
			if _, ok := def.([]string); !ok {
				t.Errorf("knob %q: multiselect default is %T, want []string", k.Key, def)
			}
		}
	}
}

func TestValidateValue_RejectsUnknownKey(t *testing.T) {
	_, err := ValidateValue("no_such_key", "1")
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("got err %v, want ErrUnknownKey", err)
	}
}

func TestValidateValue_RejectsInvalidJSON(t *testing.T) {
	_, err := ValidateValue("max_turns", "not-json")
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got err %v, want ErrTypeMismatch", err)
	}
}

func TestValidateValue_IntKnob_AcceptsWholeNumber(t *testing.T) {
	warnings, err := ValidateValue("max_turns", "100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestValidateValue_IntKnob_RejectsFractional(t *testing.T) {
	_, err := ValidateValue("max_turns", "3.5")
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got err %v, want ErrTypeMismatch", err)
	}
}

func TestValidateValue_FloatKnob_AcceptsAny(t *testing.T) {
	if _, err := ValidateValue("temperature", "0.7"); err != nil {
		t.Errorf("unexpected error for 0.7: %v", err)
	}
	if _, err := ValidateValue("temperature", "1"); err != nil {
		t.Errorf("unexpected error for whole 1: %v", err)
	}
}

func TestValidateValue_ModelKnob_AcceptsKnownModel(t *testing.T) {
	// default_model moved from KnobString to KnobEnum so the UI can render a
	// dropdown of the Claude family. Empty string remains valid ("agent default").
	for _, v := range []string{`""`, `"claude-opus-4-7"`, `"claude-sonnet-4-6"`, `"claude-haiku-4-5"`} {
		if _, err := ValidateValue("default_model", v); err != nil {
			t.Errorf("unexpected error for %s: %v", v, err)
		}
	}
}

func TestValidateValue_ModelKnob_RejectsUnknown(t *testing.T) {
	_, err := ValidateValue("default_model", `"gpt-5"`)
	if err == nil {
		t.Fatalf("expected enum rejection for unknown model, got nil")
	}
}

func TestValidateValue_EnumKnob_AcceptsKnownValue(t *testing.T) {
	if _, err := ValidateValue("reasoning_effort", `"high"`); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if _, err := ValidateValue("default_agent", `"codex"`); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if _, err := ValidateValue("approval_mode", `"on-failure"`); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateValue_EnumKnob_RejectsUnknownValue(t *testing.T) {
	_, err := ValidateValue("reasoning_effort", `"turbo"`)
	if !errors.Is(err, ErrEnumOutOfRange) {
		t.Fatalf("got err %v, want ErrEnumOutOfRange", err)
	}
}

func TestValidateValue_MultiselectKnob_AcceptsStringArray(t *testing.T) {
	warnings, err := ValidateValue("allowed_tools", `["Bash","Read"]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestValidateValue_MultiselectKnob_RejectsMixedTypes(t *testing.T) {
	_, err := ValidateValue("allowed_tools", `["Bash", 7]`)
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got err %v, want ErrTypeMismatch", err)
	}
}

func TestValidateValue_SliderOverMax_ReturnsWarningNotError(t *testing.T) {
	warnings, err := ValidateValue("max_parallel", "500")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected a warning for value above slider max, got none")
	}
}

func TestValidateValue_SliderUnderMin_ReturnsWarningNotError(t *testing.T) {
	warnings, err := ValidateValue("timeout_minutes", "0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected a warning for value below slider min, got none")
	}
}

func TestValidateValue_TypeMismatch_ErrorsIsTypeMismatch(t *testing.T) {
	_, err := ValidateValue("max_turns", `"not-an-int"`)
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got err %v, want ErrTypeMismatch", err)
	}
}

func TestValidateValue_UnknownEnum_ErrorsIsEnumOutOfRange(t *testing.T) {
	_, err := ValidateValue("default_agent", `"emacs"`)
	if !errors.Is(err, ErrEnumOutOfRange) {
		t.Fatalf("got err %v, want ErrEnumOutOfRange", err)
	}
}
