package runtime

// This changes observation DATA only, never action permission or effect state.
func sanitizeFormMetadata(result *Result, private bool) {
	if len(result.Forms) > 10 {
		result.Forms = result.Forms[:10]
	}
	for i := range result.Forms {
		form := &result.Forms[i]
		ordinary := !private && len(form.Fields) <= 32
		for _, field := range form.Fields {
			if !ordinaryFormField(field.Type) {
				ordinary = false
			}
		}
		if len(form.Fields) > 32 {
			form.Fields = form.Fields[:32]
		}
		for j := range form.Fields {
			field := &form.Fields[j]
			if !ordinary || field.Type == "button" || field.Type == "submit" || field.Type == "reset" {
				field.Constraints, field.Validation = nil, nil
				continue
			}
			if c := field.Constraints; c != nil {
				if field.Type != "number" {
					c.Min, c.Max, c.Step = "", "", ""
				}
				for _, value := range []*string{&c.Min, &c.Max, &c.Step} {
					if len(*value) > 64 {
						*value = ""
					}
				}
				if c.MinLength != nil && (*c.MinLength < 0 || *c.MinLength > 1000000) {
					c.MinLength = nil
				}
				if c.MaxLength != nil && (*c.MaxLength < 0 || *c.MaxLength > 1000000) {
					c.MaxLength = nil
				}
			}
			if result.NetworkState != "frozen" || !validFormValidation(field.Validation) {
				field.Validation = nil
			}
		}
	}
}

func ordinaryFormField(kind string) bool {
	switch kind {
	case "text", "search", "number", "email", "url", "tel", "textarea", "select-one", "select-multiple", "checkbox", "radio", "submit", "button", "reset":
		return true
	}
	return false
}

func validFormValidation(v *FormFieldValidation) bool {
	if v == nil || len(v.Issues) > 10 || v.Valid != (len(v.Issues) == 0) {
		return false
	}
	seen := map[string]bool{}
	for _, issue := range v.Issues {
		switch issue {
		case "value_missing", "type_mismatch", "range_underflow", "range_overflow", "step_mismatch", "too_short", "too_long", "bad_input", "pattern_mismatch", "custom_error":
		default:
			return false
		}
		if seen[issue] {
			return false
		}
		seen[issue] = true
	}
	return true
}
