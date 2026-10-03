// httpsd <dir> <certOut.pem> <requests.log> [addr]: throwaway local HTTPS file server for the spike.
// /files/<name> serves a file. /redir/<name> answers 302 to /files/<name>, like a GitHub release asset.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	dir, certOut, reqLog := os.Args[1], os.Args[2], os.Args[3]
	addr := "127.0.0.1:8443"
	if len(os.Args) > 4 {
		addr = os.Args[4]
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "rp-spike local"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(72 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true,
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	_ = os.WriteFile(certOut, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	lf, _ := os.OpenFile(reqLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	logReq := func(r *http.Request, note string) {
		fmt.Fprintf(lf, "%s %s %s ua=%q range=%q %s\n", time.Now().Format(time.RFC3339Nano), r.Method, r.URL.Path, r.UserAgent(), r.Header.Get("Range"), note)
	}
	mux := http.NewServeMux()
	fs := http.StripPrefix("/files/", http.FileServer(http.Dir(dir)))
	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) { logReq(r, "serve"); fs.ServeHTTP(w, r) })
	mux.HandleFunc("/redir/", func(w http.ResponseWriter, r *http.Request) {
		logReq(r, "redirect")
		http.Redirect(w, r, "/files/"+strings.TrimPrefix(r.URL.Path, "/redir/"), http.StatusFound)
	})
	srv := &http.Server{Addr: addr, Handler: mux, TLSConfig: &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}}
	fmt.Println("listening", addr)
	panic(srv.ListenAndServeTLS("", ""))
}
