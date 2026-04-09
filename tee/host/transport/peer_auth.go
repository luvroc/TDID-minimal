package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
)

type PeerAuth struct {
	allow map[string]struct{}
}

func NewPeerAuth(allowList []string) *PeerAuth {
	allow := make(map[string]struct{}, len(allowList))
	for _, v := range allowList {
		n := strings.TrimSpace(v)
		if n == "" {
			continue
		}
		allow[n] = struct{}{}
	}
	return &PeerAuth{allow: allow}
}

func (a *PeerAuth) VerifyCommonName(commonName string) error {
	if a == nil || len(a.allow) == 0 {
		return nil
	}
	if _, ok := a.allow[commonName]; !ok {
		return fmt.Errorf("peer common name not in allowlist: %s", commonName)
	}
	return nil
}

type TLSFiles struct {
	CertFile   string
	KeyFile    string
	CAFile     string
	ServerName string
}

func BuildServerTLSConfig(files TLSFiles, requireClientCert bool) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(files.CertFile, files.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server cert/key failed: %w", err)
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}
	if requireClientCert {
		pool, err := loadCertPool(files.CAFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.ClientCAs = pool
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return tlsCfg, nil
}

func BuildClientTLSConfig(files TLSFiles) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(files.CertFile, files.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load client cert/key failed: %w", err)
	}
	pool, err := loadCertPool(files.CAFile)
	if err != nil {
		return nil, err
	}
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
	}
	if files.ServerName != "" {
		tlsCfg.ServerName = files.ServerName
	}
	return tlsCfg, nil
}

func loadCertPool(caFile string) (*x509.CertPool, error) {
	if strings.TrimSpace(caFile) == "" {
		return nil, fmt.Errorf("ca file is required")
	}
	raw, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read ca file failed: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(raw) {
		return nil, fmt.Errorf("append ca pem failed")
	}
	return pool, nil
}
