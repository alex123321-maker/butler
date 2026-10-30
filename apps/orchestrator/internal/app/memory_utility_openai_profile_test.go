package app

import (
	"fmt"
	"testing"

	"github.com/butler/butler/internal/budget"
)

// Trusted research composition only; never selected by model/tool arguments.
func utilityOpenAIProfile(model string) (budget.ModelPrice, int64, error) {
	switch model {
	case "gpt-6-luna":
		return budget.ModelPrice{Basis: "operator_rate", InputMicrosPerToken: 1, OutputMicrosPerToken: 1}, 2000000, nil
	case "gpt-6.1-terra":
		// Published Standard short-context input/cache-write <=2.5 USD/M,
		// output 5 USD/M. Round input up to whole micro units for admission.
		return budget.ModelPrice{Basis: "operator_rate", InputMicrosPerToken: 3, OutputMicrosPerToken: 5}, 5000000, nil
	default:
		return budget.ModelPrice{}, 0, fmt.Errorf("unsupported frozen OpenAI research model")
	}
}

func TestMemoryUtilityOpenAIProfilesAreExplicitAndConservative(t *testing.T) {
	for _, model := range []string{"gpt-6-luna", "gpt-6.1-terra"} {
		price, ceiling, err := utilityOpenAIProfile(model)
		if err != nil || price.Basis != "operator_rate" || price.InputMicrosPerToken <= 0 || price.OutputMicrosPerToken <= 0 {
			t.Fatal("invalid research price")
		}
		if int64(13)*(65536*price.InputMicrosPerToken+32768*price.OutputMicrosPerToken) > ceiling {
			t.Fatal("ceiling cannot reserve the fixed cohort")
		}
		if model == "gpt-6-luna" && (ceiling != 2000000 || price.InputMicrosPerToken != 1 || price.OutputMicrosPerToken != 1) {
			t.Fatal("closed Luna profile changed")
		}
	}
	for _, model := range []string{"", "luna", "terra", "gpt-6.1-sol", "gpt-6.1-terra ", "gpt-6.1-terra;echo"} {
		if _, _, err := utilityOpenAIProfile(model); err == nil {
			t.Fatal("unfrozen model admitted")
		}
	}
}
