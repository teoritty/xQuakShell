package main

import (
	"context"
	"testing"
)

// This runs on every vault unlock, and a build with no plugin runtime - or one assembled far enough
// to have a manager but no vault settings - is an ordinary state rather than a bug. A nil
// dereference here would turn opening the vault into a crash.
func TestMigrateConsentWithNothingWiredDoesNothing(t *testing.T) {
	ctx := context.Background()

	var absent *pluginRuntime
	absent.migrateConsent(ctx)

	(&pluginRuntime{}).migrateConsent(ctx)
}
