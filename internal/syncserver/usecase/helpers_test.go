package usecase_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"

	"xquakshell/internal/syncserver/servertest"
)

var ctx = servertest.Ctx

func isErr(err, target error) bool { return errors.Is(err, target) }

func ed25519Key() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func sign(u *servertest.User, msg []byte) []byte { return ed25519.Sign(u.DSK, msg) }
