package localmodels

import (
	"encoding/json"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Fixed Google FunctionGemma control-token grammar. Do not rely on a changing
// Ollama chat renderer/parser to infer the schema or silently normalize a batch.
func functionPrompt(text string) (string, error) {
	if !utf8.ValidString(text) || strings.ContainsAny(text, "<>") || len(text) > 2048 {
		return "", fault(Invalid)
	}
	return "<start_of_turn>developer\nYou are a model that can do function calling with the following functions" +
		"<start_function_declaration>declaration:check_system{description:<escape>Check the health of Butler system.<escape>,parameters:{properties:{},required:[],type:<escape>OBJECT<escape>}}<end_function_declaration>" +
		"<start_function_declaration>declaration:check_database{description:<escape>Check the health of Butler database.<escape>,parameters:{properties:{},required:[],type:<escape>OBJECT<escape>}}<end_function_declaration>" +
		"<start_function_declaration>declaration:read_webpage{description:<escape>Read one public webpage by its explicit absolute HTTPS URL.<escape>,parameters:{properties:{url:{description:<escape>Explicit public HTTPS URL from the command.<escape>,type:<escape>STRING<escape>}},required:[<escape>url<escape>],type:<escape>OBJECT<escape>}}<end_function_declaration><end_of_turn>\n" +
		"<start_of_turn>user\n" + text + "<end_of_turn>\n<start_of_turn>model\n", nil
}

var noArgFunction = regexp.MustCompile(`^<start_function_call>call:(check_system|check_database)\{\}<end_function_call>$`)
var urlFunction = regexp.MustCompile(`^<start_function_call>call:read_webpage\{url:<escape>([^<>]+)<escape>\}<end_function_call>$`)

func parseFunctionText(text string) (*pb.FunctionOutput, error) {
	if !utf8.ValidString(text) || len(text) > 8192 {
		return nil, fault(Protocol)
	}
	text = strings.TrimSpace(text)
	if !strings.Contains(text, "<start_function_call>") {
		return NormalizeFunction(nil)
	}
	if m := noArgFunction.FindStringSubmatch(text); m != nil {
		return NormalizeFunction([]*pb.FunctionCall{{Name: m[1], ArgumentsJson: `{}`}})
	}
	if m := urlFunction.FindStringSubmatch(text); m != nil {
		args, _ := json.Marshal(map[string]string{"url": m[1]})
		return NormalizeFunction([]*pb.FunctionCall{{Name: "read_webpage", ArgumentsJson: string(args)}})
	}
	return nil, fault(Protocol)
}
