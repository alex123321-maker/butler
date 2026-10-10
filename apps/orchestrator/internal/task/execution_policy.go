package task

const (
	PolicyNormal      = "normal"
	PolicyPrepareOnly = "prepare_only"
)

func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}
