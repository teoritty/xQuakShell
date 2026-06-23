package pluginsdk_test

import (
	"testing"

	"github.com/xquakshell/pluginsdk"
)

func TestNetClientDialRequiresClient(t *testing.T) {
	var client *pluginsdk.Client
	if _, err := client.Net().Dial("127.0.0.1", 9); err == nil {
		t.Fatal("expected error for nil client")
	}
}
