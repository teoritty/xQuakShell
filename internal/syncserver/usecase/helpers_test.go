package usecase_test

import (
	"errors"

	"xquakshell/internal/syncserver/servertest"
)

var ctx = servertest.Ctx

func isErr(err, target error) bool { return errors.Is(err, target) }
