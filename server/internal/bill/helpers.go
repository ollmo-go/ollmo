package bill

import (
	"log"

	"github.com/google/uuid"

	"ollmo/ollmo/internal/llm"
)

func newID() string { return uuid.NewString() }

func logWarn(format string, args ...any) { log.Printf(format, args...) }

// estimateAmount converts token counts to yuan using the model's configured
// per-1M-token prices. Prices default to 0, so unconfigured models cost 0.
func estimateAmount(m *llm.LLMModel, promptTokens, completionTokens int) float64 {
	if m == nil {
		return 0
	}
	return float64(promptTokens)/1_000_000*m.InputPrice +
		float64(completionTokens)/1_000_000*m.OutputPrice
}
