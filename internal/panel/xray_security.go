package panel

import (
	"context"
	"crypto/ecdh"
	"crypto/hpke"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

const xraySecurityRequestLimit = 1 << 20

type realityScanResult struct {
	Target      string   `json:"target"`
	Feasible    bool     `json:"feasible"`
	Reason      string   `json:"reason,omitempty"`
	IP          string   `json:"ip,omitempty"`
	TLSVersion  string   `json:"tlsVersion,omitempty"`
	ALPN        string   `json:"alpn,omitempty"`
	CurveID     string   `json:"curveID,omitempty"`
	CertValid   bool     `json:"certValid"`
	CertSubject string   `json:"certSubject,omitempty"`
	CertIssuer  string   `json:"certIssuer,omitempty"`
	LatencyMS   int64    `json:"latencyMs,omitempty"`
	ServerNames []string `json:"serverNames,omitempty"`
}

func (s *Server) handleRealityX25519(w http.ResponseWriter, _ *http.Request) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		writeErrorStatus(w, http.StatusInternalServerError, fmt.Errorf("generate X25519 key: %w", err))
		return
	}
	writePanelObject(w, map[string]string{
		"privateKey": base64.RawURLEncoding.EncodeToString(privateKey.Bytes()),
		"publicKey":  base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes()),
	})
}

func (s *Server) handleRealityMLDSA65(w http.ResponseWriter, _ *http.Request) {
	var seed [mldsa65.SeedSize]byte
	if _, err := rand.Read(seed[:]); err != nil {
		writeErrorStatus(w, http.StatusInternalServerError, fmt.Errorf("generate ML-DSA-65 seed: %w", err))
		return
	}
	publicKey, privateKey := mldsa65.NewKeyFromSeed(&seed)
	publicBytes, err := publicKey.MarshalBinary()
	if err != nil {
		writeErrorStatus(w, http.StatusInternalServerError, fmt.Errorf("marshal ML-DSA-65 public key: %w", err))
		return
	}
	privateBytes, err := privateKey.MarshalBinary()
	if err != nil {
		writeErrorStatus(w, http.StatusInternalServerError, fmt.Errorf("marshal ML-DSA-65 private key: %w", err))
		return
	}
	writePanelObject(w, map[string]string{
		"seed":       base64.RawURLEncoding.EncodeToString(seed[:]),
		"verify":     base64.RawURLEncoding.EncodeToString(publicBytes),
		"publicKey":  base64.RawURLEncoding.EncodeToString(publicBytes),
		"privateKey": base64.RawURLEncoding.EncodeToString(privateBytes),
	})
}

func (s *Server) handleTLSECH(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SNI string `json:"sni"`
	}
	if err := decodeSmallJSON(r, &request); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	serverName := strings.TrimSpace(request.SNI)
	if serverName == "" {
		serverName = "cloudflare-ech.com"
	}
	if len(serverName) > 255 || strings.ContainsAny(serverName, "\x00/\\") {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("invalid ECH server name"))
		return
	}
	serverKeys, configList, err := generateECH(serverName)
	if err != nil {
		writeErrorStatus(w, http.StatusInternalServerError, err)
		return
	}
	writePanelObject(w, map[string]string{
		"echServerKeys": serverKeys,
		"echConfigList": configList,
	})
}

func (s *Server) handleTLSCertHash(w http.ResponseWriter, r *http.Request) {
	var request struct {
		CertFile    string `json:"certFile"`
		CertContent string `json:"certContent"`
	}
	if err := decodeSmallJSON(r, &request); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	var raw []byte
	var err error
	if path := strings.TrimSpace(request.CertFile); path != "" {
		raw, err = os.ReadFile(path)
	} else {
		raw = []byte(strings.TrimSpace(request.CertContent))
	}
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("read certificate: %w", err))
		return
	}
	hashes, err := certificateHashes(raw)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	writePanelObject(w, hashes)
}

func (s *Server) handleTLSRemoteCertHash(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Server string `json:"server"`
	}
	if err := decodeSmallJSON(r, &request); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	address, serverName, err := normalizeTLSTarget(request.Server, "443")
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	connection, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
	})
	if err != nil {
		writeErrorStatus(w, http.StatusBadGateway, fmt.Errorf("TLS probe failed: %w", err))
		return
	}
	defer connection.Close()
	hashes := make([]string, 0, len(connection.ConnectionState().PeerCertificates))
	for _, certificate := range connection.ConnectionState().PeerCertificates {
		sum := sha256.Sum256(certificate.Raw)
		hashes = append(hashes, hex.EncodeToString(sum[:]))
	}
	writePanelObject(w, hashes)
}

func (s *Server) handleRealityScanTarget(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Target string `json:"target"`
	}
	if err := decodeSmallJSON(r, &request); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	result := scanRealityTarget(r.Context(), request.Target)
	writePanelObject(w, result)
}

func (s *Server) handleRealityScanTargets(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Targets string `json:"targets"`
	}
	if err := decodeSmallJSON(r, &request); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	targets, err := expandRealityTargets(request.Targets, 64)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	results := make([]realityScanResult, 0, len(targets))
	for _, target := range targets {
		results = append(results, scanRealityTarget(r.Context(), target))
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Feasible != results[j].Feasible {
			return results[i].Feasible
		}
		return results[i].LatencyMS < results[j].LatencyMS
	})
	writePanelObject(w, results)
}

func writePanelObject(w http.ResponseWriter, object any) {
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": object})
}

func decodeSmallJSON(r *http.Request, target any) error {
	reader := http.MaxBytesReader(nil, r.Body, xraySecurityRequestLimit)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("request must contain one JSON object")
	}
	return nil
}

func generateECH(serverName string) (string, string, error) {
	if len(serverName) == 0 || len(serverName) > 255 || strings.ContainsAny(serverName, "\x00/\\") {
		return "", "", fmt.Errorf("invalid ECH server name")
	}
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate ECH key: %w", err)
	}
	publicKey := privateKey.PublicKey().Bytes()
	body := make([]byte, 0, 128+len(serverName))
	body = append(body, 0)
	body = binary.BigEndian.AppendUint16(body, hpke.DHKEM(ecdh.X25519()).ID())
	body = binary.BigEndian.AppendUint16(body, uint16(len(publicKey)))
	body = append(body, publicKey...)
	suites := [][2]uint16{
		{hpke.HKDFSHA256().ID(), hpke.AES128GCM().ID()},
		{hpke.HKDFSHA256().ID(), hpke.AES256GCM().ID()},
		{hpke.HKDFSHA256().ID(), hpke.ChaCha20Poly1305().ID()},
		{hpke.HKDFSHA384().ID(), hpke.AES128GCM().ID()},
		{hpke.HKDFSHA384().ID(), hpke.AES256GCM().ID()},
		{hpke.HKDFSHA384().ID(), hpke.ChaCha20Poly1305().ID()},
		{hpke.HKDFSHA512().ID(), hpke.AES128GCM().ID()},
		{hpke.HKDFSHA512().ID(), hpke.AES256GCM().ID()},
		{hpke.HKDFSHA512().ID(), hpke.ChaCha20Poly1305().ID()},
	}
	body = binary.BigEndian.AppendUint16(body, uint16(len(suites)*4))
	for _, suite := range suites {
		body = binary.BigEndian.AppendUint16(body, suite[0])
		body = binary.BigEndian.AppendUint16(body, suite[1])
	}
	body = append(body, 0, byte(len(serverName)))
	body = append(body, serverName...)
	body = binary.BigEndian.AppendUint16(body, 0)
	config := binary.BigEndian.AppendUint16(nil, 0xfe0d)
	config = binary.BigEndian.AppendUint16(config, uint16(len(body)))
	config = append(config, body...)
	configList := binary.BigEndian.AppendUint16(nil, uint16(len(config)))
	configList = append(configList, config...)
	serverKeys := binary.BigEndian.AppendUint16(nil, uint16(len(privateKey.Bytes())))
	serverKeys = append(serverKeys, privateKey.Bytes()...)
	serverKeys = binary.BigEndian.AppendUint16(serverKeys, uint16(len(config)))
	serverKeys = append(serverKeys, config...)
	return base64.StdEncoding.EncodeToString(serverKeys), base64.StdEncoding.EncodeToString(configList), nil
}

func certificateHashes(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("certificate is empty")
	}
	var certificates [][]byte
	rest := raw
	for {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			certificates = append(certificates, block.Bytes)
		}
		rest = next
	}
	if len(certificates) == 0 {
		certificates = append(certificates, raw)
	}
	hashes := make([]string, 0, len(certificates))
	for _, der := range certificates {
		if _, err := x509.ParseCertificate(der); err != nil {
			return nil, fmt.Errorf("parse certificate: %w", err)
		}
		sum := sha256.Sum256(der)
		hashes = append(hashes, hex.EncodeToString(sum[:]))
	}
	return hashes, nil
}

func scanRealityTarget(ctx context.Context, target string) realityScanResult {
	result := realityScanResult{Target: strings.TrimSpace(target), CurveID: "X25519"}
	address, serverName, err := normalizeTLSTarget(result.Target, "443")
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	result.Target = address
	dialer := &net.Dialer{Timeout: 7 * time.Second}
	started := time.Now()
	tlsConfig := &tls.Config{
		MinVersion:       tls.VersionTLS13,
		MaxVersion:       tls.VersionTLS13,
		ServerName:       serverName,
		NextProtos:       []string{"h2", "http/1.1"},
		CurvePreferences: []tls.CurveID{tls.X25519},
	}
	ipTarget := net.ParseIP(serverName) != nil
	if ipTarget {
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.ServerName = ""
	}
	rawConnection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		result.Reason = fmt.Sprintf("TCP 连接失败: %v", err)
		return result
	}
	connection := tls.Client(rawConnection, tlsConfig)
	err = connection.HandshakeContext(ctx)
	result.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		_ = rawConnection.Close()
		result.Reason = fmt.Sprintf("TLS 1.3 探测失败: %v", err)
		return result
	}
	defer connection.Close()
	state := connection.ConnectionState()
	result.IP = remoteIP(connection.RemoteAddr())
	result.TLSVersion = tlsVersionName(state.Version)
	result.ALPN = state.NegotiatedProtocol
	if len(state.PeerCertificates) > 0 {
		leaf := state.PeerCertificates[0]
		result.CertSubject = leaf.Subject.String()
		result.CertIssuer = leaf.Issuer.String()
		result.ServerNames = append([]string(nil), leaf.DNSNames...)
		result.CertValid = !ipTarget || verifyDiscoveredCertificate(leaf, state.PeerCertificates[1:])
	}
	switch {
	case state.Version != tls.VersionTLS13:
		result.Reason = "目标未协商 TLS 1.3"
	case state.NegotiatedProtocol != "h2":
		result.Reason = "目标未协商 h2 ALPN"
	case !result.CertValid:
		result.Reason = "目标证书无法通过系统信任链验证"
	default:
		result.Feasible = true
	}
	return result
}

func normalizeTLSTarget(raw, defaultPort string) (string, string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", "", fmt.Errorf("目标不能为空")
	}
	if strings.HasPrefix(target, "/") || strings.HasPrefix(target, "@") {
		return "", "", fmt.Errorf("TLS 探测不支持 Unix socket 目标")
	}
	if _, err := strconv.ParseUint(target, 10, 16); err == nil {
		target = net.JoinHostPort("127.0.0.1", target)
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		if strings.Count(target, ":") > 1 && net.ParseIP(strings.Trim(target, "[]")) != nil {
			host = strings.Trim(target, "[]")
			port = defaultPort
		} else if !strings.Contains(target, ":") {
			host = target
			port = defaultPort
		} else {
			return "", "", fmt.Errorf("目标必须是 host:port")
		}
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return "", "", fmt.Errorf("端口必须在 1-65535 范围内")
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" || strings.ContainsAny(host, "\x00/\\") {
		return "", "", fmt.Errorf("目标主机无效")
	}
	return net.JoinHostPort(host, port), host, nil
}

func verifyDiscoveredCertificate(leaf *x509.Certificate, chain []*x509.Certificate) bool {
	if len(leaf.DNSNames) == 0 {
		return false
	}
	intermediates := x509.NewCertPool()
	for _, certificate := range chain {
		intermediates.AddCert(certificate)
	}
	_, err := leaf.Verify(x509.VerifyOptions{DNSName: leaf.DNSNames[0], Intermediates: intermediates})
	return err == nil
}

func expandRealityTargets(raw string, limit int) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{"www.cloudflare.com:443", "www.microsoft.com:443", "www.amazon.com:443"}, nil
	}
	var targets []string
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' }) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(token); err == nil {
			for address := network.IP.Mask(network.Mask); network.Contains(address) && len(targets) < limit; address = nextIP(address) {
				targets = append(targets, net.JoinHostPort(address.String(), "443"))
			}
		} else {
			targets = append(targets, token)
		}
		if len(targets) >= limit {
			break
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("没有可探测的目标")
	}
	return targets, nil
}

func nextIP(ip net.IP) net.IP {
	next := append(net.IP(nil), ip...)
	for index := len(next) - 1; index >= 0; index-- {
		next[index]++
		if next[index] != 0 {
			break
		}
	}
	return next
}

func remoteIP(address net.Addr) string {
	if tcpAddress, ok := address.(*net.TCPAddr); ok {
		return tcpAddress.IP.String()
	}
	host, _, _ := net.SplitHostPort(address.String())
	return host
}

func tlsVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	default:
		return fmt.Sprintf("0x%04x", version)
	}
}
