package notify

import (
	"strings"
	"testing"
)

func TestBuildScriptEscapesAppleScriptStrings(t *testing.T) {
	got := buildScript(`Focus "check"`, "line one\npath \\ tmp")
	if !strings.Contains(got, `with title "Focus \"check\""`) {
		t.Errorf("title was not escaped: %s", got)
	}
	if !strings.Contains(got, `display notification "line one\npath \\ tmp"`) {
		t.Errorf("body was not escaped: %s", got)
	}
}
