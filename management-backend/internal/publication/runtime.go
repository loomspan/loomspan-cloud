package publication

import "errors"

const (
	TraceNever   = "NEVER"
	TraceOnError = "ONERROR"
	TraceAlways  = "ALWAYS"
)

type RuntimeSettings struct {
	Session        SessionSettings `json:"session"`
	ExecutionTrace TraceSettings   `json:"execution-trace"`
}

type SessionSettings struct {
	MissionTimeout string             `json:"mission-timeout"`
	MaxDepth       int                `json:"max-depth"`
	Attachments    AttachmentSettings `json:"attachments"`
	Quotas         QuotaSettings      `json:"quotas"`
}

type AttachmentSettings struct {
	MaxSize string `json:"max-size"`
}

type QuotaSettings struct {
	MaxSkillInvocations int `json:"max-skill-invocations"`
	MaxToolInvocations  int `json:"max-tool-invocations"`
	MaxLinterRetries    int `json:"max-linter-retries"`
	MaxModelCalls       int `json:"max-model-calls"`
	MaxProviderAttempts int `json:"max-provider-attempts"`
	MaxUsageUnits       int `json:"max-usage-units"`
}

type TraceSettings struct {
	Persistence string `json:"persistence"`
}

func EffectiveRuntimeSettings(savedTracePolicy string) (RuntimeSettings, error) {
	if savedTracePolicy == "" {
		savedTracePolicy = TraceOnError
	}
	switch savedTracePolicy {
	case TraceNever, TraceOnError, TraceAlways:
	default:
		return RuntimeSettings{}, errors.New("invalid saved execution-trace.persistence")
	}
	return RuntimeSettings{
		Session: SessionSettings{
			MissionTimeout: "60s",
			MaxDepth:       32,
			Attachments:    AttachmentSettings{MaxSize: "20971520B"},
			Quotas: QuotaSettings{
				MaxSkillInvocations: 64,
				MaxToolInvocations:  128,
				MaxLinterRetries:    32,
				MaxModelCalls:       64,
				MaxProviderAttempts: 192,
				MaxUsageUnits:       200_000,
			},
		},
		ExecutionTrace: TraceSettings{Persistence: savedTracePolicy},
	}, nil
}
