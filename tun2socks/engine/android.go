// android.go - Android绑定层
package proxyclient

import (
	"fmt"
	"sync"
)

// AndroidProxyClient Android使用的代理客户端包装器
type AndroidProxyClient struct {
	client      *ProxyClient
	mu          sync.Mutex
	logCallback LogCallback
}

// LogCallback 日志回调接口（供Java调用）
type LogCallback interface {
	OnLog(level string, message string)
}

// NewAndroidProxyClient 创建Android代理客户端
func NewAndroidProxyClient(serverAddr, serverIP, token, dnsServer, echDomain string) (*AndroidProxyClient, error) {
	config := Config{
		ServerAddr: serverAddr,
		ServerIP:   serverIP,
		Token:      token,
		DNSServer:  dnsServer,
		ECHDomain:  echDomain,
	}
	
	if config.DNSServer == "" {
		config.DNSServer = "dns.alidns.com/dns-query"
	}
	
	if config.ECHDomain == "" {
		config.ECHDomain = "cloudflare-ech.com"
	}
	
	client, err := NewProxyClient(config)
	if err != nil {
		return nil, err
	}
	
	androidClient := &AndroidProxyClient{
		client: client,
	}
	
	// 设置日志回调
	client.SetLogCallback(func(level, message string) {
		if androidClient.logCallback != nil {
			androidClient.logCallback.OnLog(level, message)
		}
	})
	
	return androidClient, nil
}

// SetLogCallback 设置日志回调
func (a *AndroidProxyClient) SetLogCallback(callback LogCallback) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logCallback = callback
}

// Start 启动代理服务
func (a *AndroidProxyClient) Start(listenAddr string) error {
	if listenAddr == "" {
		listenAddr = "127.0.0.1:1080"
	}
	return a.client.Start(listenAddr)
}

// Stop 停止代理服务
func (a *AndroidProxyClient) Stop() error {
	return a.client.Stop()
}

// IsRunning 检查是否正在运行
func (a *AndroidProxyClient) IsRunning() bool {
	return a.client.IsRunning()
}

// GetStatus 获取状态信息
func (a *AndroidProxyClient) GetStatus() string {
	if a.client.IsRunning() {
		return "运行中"
	}
	return "已停止"
}

// GetVersion 获取版本信息
func GetVersion() string {
	return "1.0.0"
}

// ======================== 简化的工厂方法 ========================

// CreateClient 简化的创建方法（使用默认参数）
func CreateClient(serverAddr, token string) (*AndroidProxyClient, error) {
	return NewAndroidProxyClient(serverAddr, "", token, "", "")
}

// CreateClientWithIP 创建客户端（指定IP）
func CreateClientWithIP(serverAddr, serverIP, token string) (*AndroidProxyClient, error) {
	return NewAndroidProxyClient(serverAddr, serverIP, token, "", "")
}

// CreateClientFull 创建客户端（完整参数）
func CreateClientFull(serverAddr, serverIP, token, dnsServer, echDomain string) (*AndroidProxyClient, error) {
	return NewAndroidProxyClient(serverAddr, serverIP, token, dnsServer, echDomain)
}

// ======================== 工具函数 ========================

// ValidateServerAddr 验证服务器地址格式
func ValidateServerAddr(addr string) error {
	if addr == "" {
		return fmt.Errorf("服务器地址不能为空")
	}
	
	_, _, _, err := parseServerAddr(addr)
	if err != nil {
		return fmt.Errorf("服务器地址格式错误: %v", err)
	}
	
	return nil
}

// TestConnection 测试连接（不启动代理服务器）
func (a *AndroidProxyClient) TestConnection() error {
	// 尝试获取ECH配置
	if err := a.client.prepareECH(); err != nil {
		return fmt.Errorf("获取ECH配置失败: %v", err)
	}
	
	// 尝试建立WebSocket连接
	wsConn, err := a.client.dialWebSocketWithECH(1)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %v", err)
	}
	defer wsConn.Close()
	
	return nil
}

// ======================== go.mod 文件内容 ========================
/*
module github.com/yourusername/proxyclient

go 1.21

require (
	github.com/gorilla/websocket v1.5.1
)
*/

// ======================== 编译说明 ========================
/*
编译为 Android AAR 的步骤：

1. 安装 gomobile:
   go install golang.org/x/mobile/cmd/gomobile@latest
   go install golang.org/x/mobile/cmd/gobind@latest

2. 初始化 gomobile:
   gomobile init

3. 编译 AAR:
   gomobile bind -target=android -androidapi=21 -o proxyclient.aar .

4. 生成的文件:
   - proxyclient.aar : Android 库文件
   - proxyclient-sources.jar : 源代码

5. 在 Android 项目中使用:
   
   a. 将 proxyclient.aar 复制到 Android 项目的 app/libs/ 目录
   
   b. 在 app/build.gradle 中添加:
      dependencies {
          implementation files('libs/proxyclient.aar')
      }
   
   c. Java/Kotlin 使用示例:

   // Java 示例
   import proxyclient.Proxyclient;
   import proxyclient.AndroidProxyClient;
   import proxyclient.LogCallback;

   public class ProxyService {
       private AndroidProxyClient client;
       
       public void start() throws Exception {
           // 创建客户端
           client = Proxyclient.createClient(
               "your-worker.workers.dev:443",
               "your-token"
           );
           
           // 设置日志回调
           client.setLogCallback(new LogCallback() {
               @Override
               public void onLog(String level, String message) {
                   Log.d("Proxy", level + ": " + message);
               }
           });
           
           // 启动代理
           client.start("127.0.0.1:1080");
       }
       
       public void stop() throws Exception {
           if (client != null) {
               client.stop();
           }
       }
       
       public boolean isRunning() {
           return client != null && client.isRunning();
       }
   }

   // Kotlin 示例
   import proxyclient.Proxyclient
   import proxyclient.AndroidProxyClient
   import proxyclient.LogCallback

   class ProxyService {
       private var client: AndroidProxyClient? = null
       
       fun start() {
           // 创建客户端
           client = Proxyclient.createClient(
               "your-worker.workers.dev:443",
               "your-token"
           )
           
           // 设置日志回调
           client?.setLogCallback(object : LogCallback {
               override fun onLog(level: String, message: String) {
                   Log.d("Proxy", "$level: $message")
               }
           })
           
           // 启动代理
           client?.start("127.0.0.1:1080")
       }
       
       fun stop() {
           client?.stop()
       }
       
       fun isRunning(): Boolean {
           return client?.isRunning() ?: false
       }
   }

6. 注意事项:
   - 需要网络权限: <uses-permission android:name="android.permission.INTERNET" />
   - 建议在 Service 中运行代理
   - 建议使用 VpnService 创建本地 VPN 连接到代理
   - 需要处理应用生命周期，确保正确启动和停止代理

7. VpnService 集成示例:

   // 在 AndroidManifest.xml 中声明
   <service
       android:name=".ProxyVpnService"
       android:permission="android.permission.BIND_VPN_SERVICE">
       <intent-filter>
           <action android:name="android.net.VpnService"/>
       </intent-filter>
   </service>

   // Java VpnService 实现
   public class ProxyVpnService extends VpnService {
       private AndroidProxyClient proxyClient;
       private ParcelFileDescriptor vpnInterface;
       
       @Override
       public int onStartCommand(Intent intent, int flags, int startId) {
           try {
               // 创建代理客户端
               proxyClient = Proxyclient.createClient(
                   "your-worker.workers.dev:443",
                   "your-token"
               );
               
               // 启动代理
               proxyClient.start("127.0.0.1:1080");
               
               // 建立 VPN 接口
               Builder builder = new Builder();
               builder.setSession("ProxyVPN")
                   .setMtu(1500)
                   .addAddress("10.0.0.2", 24)
                   .addRoute("0.0.0.0", 0)
                   .addDnsServer("8.8.8.8");
               
               vpnInterface = builder.establish();
               
               // TODO: 实现数据包转发到 SOCKS5 代理
               
           } catch (Exception e) {
               Log.e("VPN", "启动失败", e);
               stopSelf();
           }
           
           return START_STICKY;
       }
       
       @Override
       public void onDestroy() {
           try {
               if (proxyClient != null) {
                   proxyClient.stop();
               }
               if (vpnInterface != null) {
                   vpnInterface.close();
               }
           } catch (Exception e) {
               Log.e("VPN", "停止失败", e);
           }
           super.onDestroy();
       }
   }

8. 性能优化建议:
   - 使用连接池减少 WebSocket 连接开销
   - 实现流量统计和限速
   - 添加断线重连机制
   - 使用 WorkManager 管理后台任务
   - 实现电池优化和前台服务通知

9. 调试技巧:
   - 使用 adb logcat 查看日志
   - 使用 Chrome DevTools 调试 WebSocket 连接
   - 使用 Wireshark 抓包分析网络流量
   - 在开发环境使用详细日志级别

10. 常见问题解决:
    - 如果编译失败，检查 Go 版本 (需要 1.21+)
    - 如果运行时崩溃，检查权限配置
    - 如果连接失败，检查防火墙和网络策略
    - 如果性能不佳，考虑使用 NDK 优化关键路径
*/

// ======================== Makefile 示例 ========================
/*
# Makefile
.PHONY: all clean android ios

all: android

# 安装依赖
setup:
	go install golang.org/x/mobile/cmd/gomobile@latest
	go install golang.org/x/mobile/cmd/gobind@latest
	gomobile init

# 编译 Android AAR
android:
	gomobile bind -target=android -androidapi=21 -o proxyclient.aar -v .

# 编译 iOS Framework
ios:
	gomobile bind -target=ios -o ProxyClient.xcframework -v .

# 清理
clean:
	rm -f proxyclient.aar proxyclient-sources.jar
	rm -rf ProxyClient.xcframework

# 测试
test:
	go test -v ./...

# 格式化代码
fmt:
	go fmt ./...

# 检查代码
lint:
	golangci-lint run
*/
