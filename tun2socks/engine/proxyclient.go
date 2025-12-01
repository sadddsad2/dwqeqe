package proxyclient

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ProxyClient 代理客户端
type ProxyClient struct {
	serverAddr string
	serverIP   string
	token      string
	dnsServer  string
	echDomain  string
	
	echListMu sync.RWMutex
	echList   []byte
	
	listener  net.Listener
	running   bool
	mu        sync.Mutex
	
	logCallback func(level, message string)
}

// Config 配置结构
type Config struct {
	ServerAddr string // 服务端地址 (格式: x.x.workers.dev:443)
	ServerIP   string // 指定服务端IP(可选)
	Token      string // 身份验证令牌
	DNSServer  string // DNS服务器 (默认: dns.alidns.com/dns-query)
	ECHDomain  string // ECH查询域名 (默认: cloudflare-ech.com)
}

// NewProxyClient 创建新的代理客户端
func NewProxyClient(config Config) (*ProxyClient, error) {
	if config.ServerAddr == "" {
		return nil, errors.New("必须指定服务端地址")
	}
	
	if config.DNSServer == "" {
		config.DNSServer = "dns.alidns.com/dns-query"
	}
	
	if config.ECHDomain == "" {
		config.ECHDomain = "cloudflare-ech.com"
	}
	
	client := &ProxyClient{
		serverAddr: config.ServerAddr,
		serverIP:   config.ServerIP,
		token:      config.Token,
		dnsServer:  config.DNSServer,
		echDomain:  config.ECHDomain,
	}
	
	return client, nil
}

// SetLogCallback 设置日志回调
func (c *ProxyClient) SetLogCallback(callback func(level, message string)) {
	c.logCallback = callback
}

// logInfo 记录信息日志
func (c *ProxyClient) logInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if c.logCallback != nil {
		c.logCallback("INFO", msg)
	} else {
		log.Printf("[INFO] %s", msg)
	}
}

// logError 记录错误日志
func (c *ProxyClient) logError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if c.logCallback != nil {
		c.logCallback("ERROR", msg)
	} else {
		log.Printf("[ERROR] %s", msg)
	}
}

// Start 启动代理服务器
func (c *ProxyClient) Start(listenAddr string) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return errors.New("代理服务器已在运行")
	}
	c.mu.Unlock()
	
	c.logInfo("正在获取 ECH 配置...")
	if err := c.prepareECH(); err != nil {
		return fmt.Errorf("获取 ECH 配置失败: %w", err)
	}
	
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("监听失败: %w", err)
	}
	
	c.mu.Lock()
	c.listener = listener
	c.running = true
	c.mu.Unlock()
	
	c.logInfo("代理服务器启动: %s (支持 SOCKS5 和 HTTP)", listenAddr)
	c.logInfo("后端服务器: %s", c.serverAddr)
	if c.serverIP != "" {
		c.logInfo("使用固定 IP: %s", c.serverIP)
	}
	
	go c.acceptLoop()
	
	return nil
}

// Stop 停止代理服务器
func (c *ProxyClient) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if !c.running {
		return errors.New("代理服务器未运行")
	}
	
	c.running = false
	if c.listener != nil {
		c.listener.Close()
		c.listener = nil
	}
	
	c.logInfo("代理服务器已停止")
	return nil
}

// IsRunning 检查是否正在运行
func (c *ProxyClient) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// acceptLoop 接受连接循环
func (c *ProxyClient) acceptLoop() {
	for {
		c.mu.Lock()
		listener := c.listener
		running := c.running
		c.mu.Unlock()
		
		if !running || listener == nil {
			return
		}
		
		conn, err := listener.Accept()
		if err != nil {
			if !running {
				return
			}
			c.logError("接受连接失败: %v", err)
			continue
		}
		
		go c.handleConnection(conn)
	}
}

// ======================== ECH 支持 ========================

const typeHTTPS = 65

func (c *ProxyClient) prepareECH() error {
	echBase64, err := c.queryHTTPSRecord(c.echDomain, c.dnsServer)
	if err != nil {
		return fmt.Errorf("DNS 查询失败: %w", err)
	}
	if echBase64 == "" {
		return errors.New("未找到 ECH 参数")
	}
	raw, err := base64.StdEncoding.DecodeString(echBase64)
	if err != nil {
		return fmt.Errorf("ECH 解码失败: %w", err)
	}
	c.echListMu.Lock()
	c.echList = raw
	c.echListMu.Unlock()
	c.logInfo("ECH 配置已加载，长度: %d 字节", len(raw))
	return nil
}

func (c *ProxyClient) refreshECH() error {
	c.logInfo("刷新 ECH 配置...")
	return c.prepareECH()
}

func (c *ProxyClient) getECHList() ([]byte, error) {
	c.echListMu.RLock()
	defer c.echListMu.RUnlock()
	if len(c.echList) == 0 {
		return nil, errors.New("ECH 配置未加载")
	}
	return c.echList, nil
}

func (c *ProxyClient) buildTLSConfigWithECH(serverName string, echList []byte) (*tls.Config, error) {
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("加载系统根证书失败: %w", err)
	}
	return &tls.Config{
		MinVersion:                     tls.VersionTLS13,
		ServerName:                     serverName,
		EncryptedClientHelloConfigList: echList,
		EncryptedClientHelloRejectionVerify: func(cs tls.ConnectionState) error {
			return errors.New("服务器拒绝 ECH")
		},
		RootCAs: roots,
	}, nil
}

// ======================== DNS 查询 ========================

func (c *ProxyClient) queryHTTPSRecord(domain, dnsServer string) (string, error) {
	if _, _, err := net.SplitHostPort(dnsServer); err == nil {
		return c.queryHTTPSRecordUDP(domain, dnsServer)
	}
	
	dohURL := dnsServer
	if !strings.HasPrefix(dohURL, "http://") && !strings.HasPrefix(dohURL, "https://") {
		dohURL = "https://" + dohURL
	}
	
	return c.queryHTTPSRecordDoH(domain, dohURL)
}

func (c *ProxyClient) queryHTTPSRecordDoH(domain, dohURL string) (string, error) {
	query := buildDNSQuery(domain, typeHTTPS)
	
	req, err := http.NewRequest("POST", dohURL, bytes.NewReader(query))
	if err != nil {
		return "", fmt.Errorf("创建DoH请求失败: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")
	
	u, err := url.Parse(dohURL)
	if err != nil {
		return "", fmt.Errorf("解析DoH URL失败: %w", err)
	}
	
	host := u.Hostname()
	isIP := net.ParseIP(host) != nil
	
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	
	if isIP {
		targetIP := net.ParseIP(host)
		
		tlsConfig.InsecureSkipVerify = false
		tlsConfig.VerifyConnection = func(cs tls.ConnectionState) error {
			cert := cs.PeerCertificates[0]
			
			ipMatched := false
			for _, certIP := range cert.IPAddresses {
				if certIP.Equal(targetIP) {
					ipMatched = true
					break
				}
			}
			
			if !ipMatched {
				return fmt.Errorf("证书不包含目标IP: %s", host)
			}
			
			opts := x509.VerifyOptions{
				Intermediates: x509.NewCertPool(),
			}
			
			for _, intermediateCert := range cs.PeerCertificates[1:] {
				opts.Intermediates.AddCert(intermediateCert)
			}
			
			if _, err := cert.Verify(opts); err != nil {
				return fmt.Errorf("证书链验证失败: %w", err)
			}
			
			return nil
		}
	}
	
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("DoH请求失败: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DoH响应错误: %d", resp.StatusCode)
	}
	
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取DoH响应失败: %w", err)
	}
	
	return parseDNSResponse(response)
}

func (c *ProxyClient) queryHTTPSRecordUDP(domain, dnsServer string) (string, error) {
	query := buildDNSQuery(domain, typeHTTPS)

	conn, err := net.Dial("udp", dnsServer)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if _, err = conn.Write(query); err != nil {
		return "", err
	}

	response := make([]byte, 4096)
	n, err := conn.Read(response)
	if err != nil {
		return "", err
	}
	return parseDNSResponse(response[:n])
}

func buildDNSQuery(domain string, qtype uint16) []byte {
	query := make([]byte, 0, 512)
	query = append(query, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
	for _, label := range strings.Split(domain, ".") {
		query = append(query, byte(len(label)))
		query = append(query, []byte(label)...)
	}
	query = append(query, 0x00, byte(qtype>>8), byte(qtype), 0x00, 0x01)
	return query
}

func parseDNSResponse(response []byte) (string, error) {
	if len(response) < 12 {
		return "", errors.New("响应过短")
	}
	ancount := binary.BigEndian.Uint16(response[6:8])
	if ancount == 0 {
		return "", errors.New("无应答记录")
	}

	offset := 12
	for offset < len(response) && response[offset] != 0 {
		offset += int(response[offset]) + 1
	}
	offset += 5

	for i := 0; i < int(ancount); i++ {
		if offset >= len(response) {
			break
		}
		if response[offset]&0xC0 == 0xC0 {
			offset += 2
		} else {
			for offset < len(response) && response[offset] != 0 {
				offset += int(response[offset]) + 1
			}
			offset++
		}
		if offset+10 > len(response) {
			break
		}
		rrType := binary.BigEndian.Uint16(response[offset : offset+2])
		offset += 8
		dataLen := binary.BigEndian.Uint16(response[offset : offset+2])
		offset += 2
		if offset+int(dataLen) > len(response) {
			break
		}
		data := response[offset : offset+int(dataLen)]
		offset += int(dataLen)

		if rrType == typeHTTPS {
			if ech := parseHTTPSRecord(data); ech != "" {
				return ech, nil
			}
		}
	}
	return "", nil
}

func parseHTTPSRecord(data []byte) string {
	if len(data) < 2 {
		return ""
	}
	offset := 2
	if offset < len(data) && data[offset] == 0 {
		offset++
	} else {
		for offset < len(data) && data[offset] != 0 {
			offset += int(data[offset]) + 1
		}
		offset++
	}
	for offset+4 <= len(data) {
		key := binary.BigEndian.Uint16(data[offset : offset+2])
		length := binary.BigEndian.Uint16(data[offset+2 : offset+4])
		offset += 4
		if offset+int(length) > len(data) {
			break
		}
		value := data[offset : offset+int(length)]
		offset += int(length)
		if key == 5 {
			return base64.StdEncoding.EncodeToString(value)
		}
	}
	return ""
}

// ======================== 工具函数 ========================

func isNormalCloseError(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, "use of closed network connection") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "normal closure")
}

// ======================== WebSocket 客户端 ========================

func parseServerAddr(addr string) (host, port, path string, err error) {
	path = "/"
	slashIdx := strings.Index(addr, "/")
	if slashIdx != -1 {
		path = addr[slashIdx:]
		addr = addr[:slashIdx]
	}

	host, port, err = net.SplitHostPort(addr)
	if err != nil {
		return "", "", "", fmt.Errorf("无效的服务器地址格式: %v", err)
	}

	return host, port, path, nil
}

func (c *ProxyClient) dialWebSocketWithECH(maxRetries int) (*websocket.Conn, error) {
	host, port, path, err := parseServerAddr(c.serverAddr)
	if err != nil {
		return nil, err
	}

	wsURL := fmt.Sprintf("wss://%s:%s%s", host, port, path)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		echBytes, echErr := c.getECHList()
		if echErr != nil {
			if attempt < maxRetries {
				c.refreshECH()
				continue
			}
			return nil, echErr
		}

		tlsCfg, tlsErr := c.buildTLSConfigWithECH(host, echBytes)
		if tlsErr != nil {
			return nil, tlsErr
		}

		dialer := websocket.Dialer{
			TLSClientConfig: tlsCfg,
			Subprotocols: func() []string {
				if c.token == "" {
					return nil
				}
				return []string{c.token}
			}(),
			HandshakeTimeout: 10 * time.Second,
		}

		if c.serverIP != "" {
			dialer.NetDial = func(network, address string) (net.Conn, error) {
				_, port, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				return net.DialTimeout(network, net.JoinHostPort(c.serverIP, port), 10*time.Second)
			}
		}

		wsConn, _, dialErr := dialer.Dial(wsURL, nil)
		if dialErr != nil {
			if strings.Contains(dialErr.Error(), "ECH") && attempt < maxRetries {
				c.logInfo("ECH 连接失败，尝试刷新配置 (%d/%d)", attempt, maxRetries)
				c.refreshECH()
				time.Sleep(time.Second)
				continue
			}
			return nil, dialErr
		}

		return wsConn, nil
	}

	return nil, errors.New("连接失败，已达最大重试次数")
}

// ======================== 连接处理 ========================

func (c *ProxyClient) handleConnection(conn net.Conn) {
	defer conn.Close()

	clientAddr := conn.RemoteAddr().String()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return
	}

	firstByte := buf[0]

	switch firstByte {
	case 0x05:
		c.handleSOCKS5(conn, clientAddr, firstByte)
	case 'C', 'G', 'P', 'H', 'D', 'O', 'T':
		c.handleHTTP(conn, clientAddr, firstByte)
	default:
		c.logError("未知协议: 0x%02x from %s", firstByte, clientAddr)
	}
}

// ======================== SOCKS5 处理 ========================

func (c *ProxyClient) handleSOCKS5(conn net.Conn, clientAddr string, firstByte byte) {
	if firstByte != 0x05 {
		c.logError("SOCKS5 版本错误: 0x%02x from %s", firstByte, clientAddr)
		return
	}

	buf := make([]byte, 1)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}

	nmethods := buf[0]
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}

	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	buf = make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}

	if buf[0] != 5 {
		return
	}

	command := buf[1]
	atyp := buf[3]

	var host string
	switch atyp {
	case 0x01:
		buf = make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		host = net.IP(buf).String()

	case 0x03:
		buf = make([]byte, 1)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		domainBuf := make([]byte, buf[0])
		if _, err := io.ReadFull(conn, domainBuf); err != nil {
			return
		}
		host = string(domainBuf)

	case 0x04:
		buf = make([]byte, 16)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		host = net.IP(buf).String()

	default:
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return
	}

	buf = make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}
	port := int(buf[0])<<8 | int(buf[1])

	var target string
	if atyp == 0x04 {
		target = fmt.Sprintf("[%s]:%d", host, port)
	} else {
		target = fmt.Sprintf("%s:%d", host, port)
	}

	if command != 0x01 {
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return
	}

	c.logInfo("SOCKS5: %s -> %s", clientAddr, target)

	if err := c.handleTunnel(conn, target, clientAddr, modeSOCKS5, ""); err != nil {
		if !isNormalCloseError(err) {
			c.logError("SOCKS5 代理失败 %s: %v", clientAddr, err)
		}
	}
}

// ======================== HTTP 处理 ========================

func (c *ProxyClient) handleHTTP(conn net.Conn, clientAddr string, firstByte byte) {
	reader := bufio.NewReader(io.MultiReader(
		strings.NewReader(string(firstByte)),
		conn,
	))

	requestLine, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	parts := strings.Fields(requestLine)
	if len(parts) < 3 {
		return
	}

	method := parts[0]
	requestURL := parts[1]
	httpVersion := parts[2]

	headers := make(map[string]string)
	var headerLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		headerLines = append(headerLines, line)
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			headers[strings.ToLower(key)] = value
		}
	}

	switch method {
	case "CONNECT":
		c.logInfo("HTTP-CONNECT: %s -> %s", clientAddr, requestURL)
		if err := c.handleTunnel(conn, requestURL, clientAddr, modeHTTPConnect, ""); err != nil {
			if !isNormalCloseError(err) {
				c.logError("HTTP-CONNECT 代理失败 %s: %v", clientAddr, err)
			}
		}

	case "GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "TRACE":
		c.logInfo("HTTP-%s: %s -> %s", method, clientAddr, requestURL)

		var target string
		var path string

		if strings.HasPrefix(requestURL, "http://") {
			urlWithoutScheme := strings.TrimPrefix(requestURL, "http://")
			idx := strings.Index(urlWithoutScheme, "/")
			if idx > 0 {
				target = urlWithoutScheme[:idx]
				path = urlWithoutScheme[idx:]
			} else {
				target = urlWithoutScheme
				path = "/"
			}
		} else {
			target = headers["host"]
			path = requestURL
		}

		if target == "" {
			conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			return
		}

		if !strings.Contains(target, ":") {
			target += ":80"
		}

		var requestBuilder strings.Builder
		requestBuilder.WriteString(fmt.Sprintf("%s %s %s\r\n", method, path, httpVersion))

		for _, line := range headerLines {
			key := strings.Split(line, ":")[0]
			keyLower := strings.ToLower(strings.TrimSpace(key))
			if keyLower != "proxy-connection" && keyLower != "proxy-authorization" {
				requestBuilder.WriteString(line)
				requestBuilder.WriteString("\r\n")
			}
		}
		requestBuilder.WriteString("\r\n")

		if contentLength := headers["content-length"]; contentLength != "" {
			var length int
			fmt.Sscanf(contentLength, "%d", &length)
			if length > 0 && length < 10*1024*1024 {
				body := make([]byte, length)
				if _, err := io.ReadFull(reader, body); err == nil {
					requestBuilder.Write(body)
				}
			}
		}

		firstFrame := requestBuilder.String()

		if err := c.handleTunnel(conn, target, clientAddr, modeHTTPProxy, firstFrame); err != nil {
			if !isNormalCloseError(err) {
				c.logError("HTTP-%s 代理失败 %s: %v", method, clientAddr, err)
			}
		}

	default:
		c.logInfo("HTTP 不支持的方法: %s from %s", method, clientAddr)
		conn.Write([]byte("HTTP/1.1 405 Method Not Allowed\r\n\r\n"))
	}
}

// ======================== 通用隧道处理 ========================

const (
	modeSOCKS5      = 1
	modeHTTPConnect = 2
	modeHTTPProxy   = 3
)

func (c *ProxyClient) handleTunnel(conn net.Conn, target, clientAddr string, mode int, firstFrame string) error {
	wsConn, err := c.dialWebSocketWithECH(2)
	if err != nil {
		c.sendErrorResponse(conn, mode)
		return err
	}
	defer wsConn.Close()

	var mu sync.Mutex

	stopPing := make(chan bool)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				wsConn.WriteMessage(websocket.PingMessage, nil)
				mu.Unlock()
			case <-stopPing:
				return
			}
		}
	}()
	defer close(stopPing)

	conn.SetDeadline(time.Time{})

	if firstFrame == "" && mode == modeSOCKS5 {
		_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		buffer := make([]byte, 32768)
		n, _ := conn.Read(buffer)
		_ = conn.SetReadDeadline(time.Time{})
		if n > 0 {
			firstFrame = string(buffer[:n])
		}
	}

	connectMsg := fmt.Sprintf("CONNECT:%s|%s", target, firstFrame)
	mu.Lock()
	err = wsConn.WriteMessage(websocket.TextMessage, []byte(connectMsg))
	mu.Unlock()
	if err != nil {
		c.sendErrorResponse(conn, mode)
		return err
	}

	_, msg, err := wsConn.ReadMessage()
	if err != nil {
		c.sendErrorResponse(conn, mode)
		return err
	}

	response := string(msg)
	if strings.HasPrefix(response, "ERROR:") {
		c.sendErrorResponse(conn, mode)
		return errors.New(response)
	}
	if response != "CONNECTED" {
		c.sendErrorResponse(conn, mode)
		return fmt.Errorf("意外响应: %s", response)
	}

	if err := c.sendSuccessResponse(conn, mode); err != nil {
		return err
	}

	c.logInfo("已连接: %s -> %s", clientAddr, target)

	done := make(chan bool, 2)

	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				mu.Lock()
				wsConn.WriteMessage(websocket.TextMessage, []byte("CLOSE"))
				mu.Unlock()
				done <- true
				return
			}

			mu.Lock()
			err = wsConn.WriteMessage(websocket.BinaryMessage, buf[:n])
			mu.Unlock()
			if err != nil {
				done <- true
				return
			}
		}
	}()

	go func() {
		for {
			mt, msg, err := wsConn.ReadMessage()
			if err != nil {
				done <- true
				return
			}

			if mt == websocket.TextMessage {
				if string(msg) == "CLOSE" {
					done <- true
					return
				}
			}

			if _, err := conn.Write(msg); err != nil {
				done <- true
				return
			}
		}
	}()

	<-done
	c.logInfo("已断开: %s -> %s", clientAddr, target)
	return nil
}

// ======================== 响应辅助函数 ========================

func (c *ProxyClient) sendErrorResponse(conn net.Conn, mode int) {
	switch mode {
	case modeSOCKS5:
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	case modeHTTPConnect, modeHTTPProxy:
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
	}
}

func (c *ProxyClient) sendSuccessResponse(conn net.Conn, mode int) error {
	switch mode {
	case modeSOCKS5:
		_, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return err
	case modeHTTPConnect:
		_, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		return err
	case modeHTTPProxy:
		return nil
	}
	return nil
}
