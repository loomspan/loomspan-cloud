package publication

import (
	"encoding/json"
	"testing"
)

func TestRuntimeSettings(t *testing.T) {
	for _, selection := range []string{"", TraceNever, TraceOnError, TraceAlways} {
		settings, err := EffectiveRuntimeSettings(selection)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(settings)
		if err != nil {
			t.Fatal(err)
		}
		wantSelection := selection
		if wantSelection == "" {
			wantSelection = TraceOnError
		}
		want := `{"session":{"mission-timeout":"60s","max-depth":32,"attachments":{"max-size":"20971520B"},"quotas":{"max-skill-invocations":64,"max-tool-invocations":128,"max-linter-retries":32,"max-model-calls":64,"max-provider-attempts":192,"max-usage-units":200000}},"execution-trace":{"persistence":"` + wantSelection + `"}}`
		if string(encoded) != want {
			t.Fatalf("got %s, want %s", encoded, want)
		}
	}
	if _, err := EffectiveRuntimeSettings("never"); err == nil {
		t.Fatal("unknown selection accepted")
	}
}
