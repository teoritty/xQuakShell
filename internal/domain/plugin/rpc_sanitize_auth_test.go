package plugin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeAuthRPCParamsRedactsSensitiveFields(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"attemptId":    "a1",
		"authMethodId": "otp",
		"dataBase64":   "c2VjcmV0",
		"answers":      []string{"123456"},
		"questions": []map[string]any{
			{"text": "Password:", "echoOn": false},
		},
	})
	out := SanitizeAuthRPCParams("auth.answerChallenge", raw)
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["dataBase64"] != "<redacted>" {
		t.Fatalf("dataBase64 = %v", parsed["dataBase64"])
	}
	if parsed["answers"] != "<redacted>" {
		t.Fatalf("answers = %v", parsed["answers"])
	}
	qs := parsed["questions"].([]any)
	q0 := qs[0].(map[string]any)
	if q0["text"] != "<redacted>" {
		t.Fatalf("question text = %v", q0["text"])
	}
}

func TestSanitizeAuthRPCParamsLeavesNonAuthUntouched(t *testing.T) {
	raw := json.RawMessage(`{"dataBase64":"c2VjcmV0"}`)
	out := SanitizeAuthRPCParams("vault.getSecret", raw)
	if string(out) != string(raw) {
		t.Fatalf("non-auth params changed: %s", out)
	}
}

func TestSanitizeAuthRPCResultRedactsSignature(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{
		"signatureBase64":     "c2VjcmV0",
		"signatureFormat":     "ssh-ed25519",
		"publicKeyBlobBase64": "AQID",
	})
	out := SanitizeAuthRPCResult("auth.sign", raw)
	if strings.Contains(string(out), "c2VjcmV0") {
		t.Fatalf("signature not redacted: %s", out)
	}
	if !strings.Contains(string(out), "redacted") {
		t.Fatalf("expected redacted marker in %s", out)
	}
}

func TestSanitizeAuthRPCResultInvalidJSONPassthrough(t *testing.T) {
	raw := json.RawMessage(`not-json`)
	out := SanitizeAuthRPCResult("auth.sign", raw)
	if string(out) != string(raw) {
		t.Fatalf("invalid JSON should pass through unchanged")
	}
}

func TestSanitizeAuthRPCResultLeavesNonAuthUntouched(t *testing.T) {
	raw := json.RawMessage(`{"signatureBase64":"c2VjcmV0"}`)
	out := SanitizeAuthRPCResult("vault.getSecret", raw)
	if string(out) != string(raw) {
		t.Fatalf("non-auth result changed: %s", out)
	}
}

func TestSanitizeAuthRPCParamsInvalidJSONPassthrough(t *testing.T) {
	raw := json.RawMessage(`not-json`)
	out := SanitizeAuthRPCParams("auth.sign", raw)
	if string(out) != string(raw) {
		t.Fatalf("invalid JSON should pass through unchanged")
	}
}

// The cases below pin the property the enumerated, two-level redactor did not
// have: a sensitive key is redacted wherever it sits, not only at the top of
// the payload. Each one leaked in clear text before the recursive redactor.

func TestSanitizeAuthRPCParamsRedactsNestedSensitiveField(t *testing.T) {
	raw := json.RawMessage(`{"attemptId":"a1","envelope":{"inner":{"dataBase64":"c2VjcmV0"}}}`)
	out := SanitizeAuthRPCParams("auth.answerChallenge", raw)
	if strings.Contains(string(out), "c2VjcmV0") {
		t.Fatalf("nested secret leaked: %s", out)
	}
	if !strings.Contains(string(out), "a1") {
		t.Fatalf("non-sensitive field lost: %s", out)
	}
}

func TestSanitizeAuthRPCParamsRedactsSensitiveFieldInsideQuestion(t *testing.T) {
	raw := json.RawMessage(`{"questions":[{"text":"Password:","dataBase64":"c2VjcmV0"}]}`)
	out := SanitizeAuthRPCParams("auth.answerChallenge", raw)
	if strings.Contains(string(out), "c2VjcmV0") {
		t.Fatalf("secret inside a question leaked: %s", out)
	}
	if strings.Contains(string(out), "Password:") {
		t.Fatalf("question text leaked: %s", out)
	}
}

func TestSanitizeAuthRPCParamsRedactsThroughNestedArrays(t *testing.T) {
	raw := json.RawMessage(`{"batch":[[{"answers":["123456"]}]]}`)
	out := SanitizeAuthRPCParams("auth.answerChallenge", raw)
	if strings.Contains(string(out), "123456") {
		t.Fatalf("secret nested in arrays leaked: %s", out)
	}
}

// A payload can nest far deeper than any real auth message. Past the bound the
// redactor stops being able to reason about what it is looking at, so it fails
// closed rather than passing the remainder through unexamined.
func TestSanitizeAuthRPCParamsFailsClosedPastDepthBound(t *testing.T) {
	var sb strings.Builder
	const depth = maxRedactDepth * 3
	for i := 0; i < depth; i++ {
		sb.WriteString(`{"n":`)
	}
	sb.WriteString(`{"dataBase64":"c2VjcmV0"}`)
	for i := 0; i < depth; i++ {
		sb.WriteString(`}`)
	}
	out := SanitizeAuthRPCParams("auth.sign", json.RawMessage(sb.String()))
	if strings.Contains(string(out), "c2VjcmV0") {
		t.Fatalf("secret past the depth bound leaked: %s", out)
	}
}

func TestSanitizeAuthRPCParamsDoesNotInventQuestionText(t *testing.T) {
	raw := json.RawMessage(`{"questions":[{"echoOn":false}]}`)
	out := SanitizeAuthRPCParams("auth.answerChallenge", raw)
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	q0 := parsed["questions"].([]any)[0].(map[string]any)
	if _, ok := q0["text"]; ok {
		t.Fatalf("text invented on a question that had none: %s", out)
	}
}

func TestSanitizeAuthRPCParamsSurvivesQuestionsOfTheWrongShape(t *testing.T) {
	raw := json.RawMessage(`{"questions":"not-an-array","attemptId":"a1"}`)
	out := SanitizeAuthRPCParams("auth.answerChallenge", raw)
	if !strings.Contains(string(out), "a1") {
		t.Fatalf("payload mangled: %s", out)
	}
}

func TestSanitizeAuthRPCParamsPassesThroughNonObjectJSON(t *testing.T) {
	for _, raw := range []string{`[1,2]`, `"str"`, `null`, `7`} {
		out := SanitizeAuthRPCParams("auth.sign", json.RawMessage(raw))
		if string(out) != raw {
			t.Fatalf("non-object JSON %s changed to %s", raw, out)
		}
	}
}
