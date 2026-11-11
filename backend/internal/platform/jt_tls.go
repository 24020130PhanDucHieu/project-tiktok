package platform

import (
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"errors"
	"net/http"
)

// jtexpress.vn serves its own certificate without the intermediate that signed it ("Sectigo Public Server
// Authentication CA DV R36"). Browsers and Windows fetch the missing certificate by themselves; Go on Linux does not
// and refuses the connection ("certificate signed by unknown authority"). The intermediate is bundled here and used
// only to complete the chain: the chain must still end at a root the system trusts, and the host name must match.
//
//go:embed jt_intermediate.pem
var jtIntermediatePEM []byte

// jtHTTPClient is an HTTP client that verifies servers as usual, with the bundled intermediate available.
func jtHTTPClient() *http.Client {
	extra := x509.NewCertPool()
	extra.AppendCertsFromPEM(jtIntermediatePEM)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		// the default check is replaced by verifyWithIntermediates below; it is not skipped
		InsecureSkipVerify: true, //nolint:gosec
		VerifyConnection: func(cs tls.ConnectionState) error {
			return verifyWithIntermediates(cs, extra)
		},
	}
	return &http.Client{Transport: tr}
}

// verifyWithIntermediates does what crypto/tls does by default (system roots, host name, the certificates the server
// sent), plus the intermediates in extra.
func verifyWithIntermediates(cs tls.ConnectionState, extra *x509.CertPool) error {
	if len(cs.PeerCertificates) == 0 {
		return errors.New("tls: server sent no certificate")
	}
	inter := extra.Clone()
	for _, c := range cs.PeerCertificates[1:] {
		inter.AddCert(c)
	}
	_, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{DNSName: cs.ServerName, Intermediates: inter})
	return err
}
