package task

const (
	PolicyNormal      = "normal"
	PolicyPrepareOnly = "prepare_only"
)

func ValidatePolicy(policy string) error {
	if policy != "" && policy != PolicyNormal && policy != PolicyPrepareOnly {
		return ErrInvalid
	}
	return nil
}

func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}
