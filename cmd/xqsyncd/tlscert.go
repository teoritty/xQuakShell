package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	certFileName = "tls.crt"
	keyPEMName   = "tls.key"
)

// writeSelfSignedCert creates a P-256 certificate for hosts. Clients do not rely on it being
// signed by anyone: they pin the server identity key, which signs this certificate's public key in
// hello, so a self-signed certificate is as trustworthy here as a CA-issued one.
func writeSelfSignedCert(dir string, hosts []string, now time.Time) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "xqsyncd"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(10, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := writePEM(filepath.Join(dir, certFileName), "CERTIFICATE", der); err != nil {
		return err
	}
	return writePEM(filepath.Join(dir, keyPEMName), "PRIVATE KEY", keyDER)
}

func writePEM(path, kind string, der []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("xqsyncd: create %s: %w", filepath.Base(path), err)
	}
	if err := pem.Encode(f, &pem.Block{Type: kind, Bytes: der}); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// loadCertificate loads the TLS pair and returns it with the DER SubjectPublicKeyInfo the identity
// key signs.
func loadCertificate(certPath, keyPath string) (tls.Certificate, []byte, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("xqsyncd: load TLS certificate: %w", err)
	}
	if len(cert.Certificate) == 0 {
		return tls.Certificate{}, nil, errors.New("xqsyncd: TLS certificate file holds no certificate")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return cert, leaf.RawSubjectPublicKeyInfo, nil
}
