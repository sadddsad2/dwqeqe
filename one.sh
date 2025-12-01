#!/bin/bash
# one_click_deploy.sh - AppProxy ECH客户端一键部署脚本
# 
# 使用方法:
#   bash <(curl -s https://raw.githubusercontent.com/ys1231/appproxy/iyue/one_click_deploy.sh)
#
# 或者:
#   curl -O https://raw.githubusercontent.com/ys1231/appproxy/iyue/one_click_deploy.sh
#   chmod +x one_click_deploy.sh
#   ./one_click_deploy.sh

set -e

# ===== 颜色和样式 =====
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# ===== 配置变量 =====
REPO_URL="https://github.com/ys1231/appproxy.git"
REPO_BRANCH="iyue"
TARGET_DIR="appproxy"

# ===== 辅助函数 =====
print_banner() {
    echo -e "${CYAN}${BOLD}"
    cat << "EOF"
    ╔═══════════════════════════════════════════════════╗
    ║                                                   ║
    ║     AppProxy ECH Client - One Click Deploy       ║
    ║                                                   ║
    ║     🚀 自动化构建和部署系统                        ║
    ║                                                   ║
    ╚═══════════════════════════════════════════════════╝
EOF
    echo -e "${NC}\n"
}

print_step() {
    echo -e "\n${PURPLE}${BOLD}▶ $1${NC}\n"
}

print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

spinner() {
    local pid=$1
    local delay=0.1
    local spinstr='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    while [ "$(ps a | awk '{print $1}' | grep $pid)" ]; do
        local temp=${spinstr#?}
        printf " [%c]  " "$spinstr"
        local spinstr=$temp${spinstr%"$temp"}
        sleep $delay
        printf "\b\b\b\b\b\b"
    done
    printf "    \b\b\b\b"
}

# ===== 环境检查 =====
check_environment() {
    print_step "检查运行环境"
    
    local errors=0
    
    # 检查操作系统
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        print_success "操作系统: Linux"
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        print_success "操作系统: macOS"
    else
        print_warning "操作系统: $OSTYPE (未完全测试)"
    fi
    
    # 检查必需工具
    local tools=("git" "go" "flutter" "curl")
    for tool in "${tools[@]}"; do
        if command_exists $tool; then
            case $tool in
                git)
                    print_success "Git: $(git --version | awk '{print $3}')"
                    ;;
                go)
                    print_success "Go: $(go version | awk '{print $3}')"
                    ;;
                flutter)
                    print_success "Flutter: $(flutter --version | head -n 1 | awk '{print $2}')"
                    ;;
                curl)
                    print_success "Curl: 已安装"
                    ;;
            esac
        else
            print_error "$tool 未安装"
            errors=$((errors + 1))
        fi
    done
    
    if [ $errors -gt 0 ]; then
        print_error "环境检查失败，请安装缺失的工具"
        echo ""
        print_info "安装指南:"
        print_info "  Git:     https://git-scm.com/downloads"
        print_info "  Go:      https://go.dev/dl/"
        print_info "  Flutter: https://flutter.dev/docs/get-started/install"
        exit 1
    fi
    
    print_success "环境检查完成"
}

# ===== 克隆或更新仓库 =====
setup_repository() {
    print_step "设置代码仓库"
    
    if [ -d "$TARGET_DIR" ]; then
        print_info "发现现有仓库，正在更新..."
        cd "$TARGET_DIR"
        git fetch origin
        git checkout $REPO_BRANCH
        git pull origin $REPO_BRANCH
        print_success "仓库已更新"
    else
        print_info "克隆仓库: $REPO_URL"
        git clone -b $REPO_BRANCH $REPO_URL $TARGET_DIR
        cd "$TARGET_DIR"
        print_success "仓库克隆完成"
    fi
}

# ===== 创建项目结构 =====
create_structure() {
    print_step "创建项目结构"
    
    local dirs=(
        "tun2socks/engine"
        "android/app/libs"
        "android/app/src/main/kotlin/com/appproxy/ech"
        "lib/services"
        "lib/pages"
        ".github/workflows"
    )
    
    for dir in "${dirs[@]}"; do
        mkdir -p "$dir"
        print_success "创建目录: $dir"
    done
}

# ===== 创建Go模块文件 =====
create_go_module() {
    print_step "创建Go模块配置"
    
    cat > tun2socks/engine/go.mod << 'EOF'
module github.com/ys1231/appproxy/tun2socks/engine

go 1.21

require (
	github.com/gorilla/websocket v1.5.1
	golang.org/x/mobile v0.0.0-20231127183840-76ac6878050a
)

require (
	golang.org/x/mod v0.14.0 // indirect
	golang.org/x/sync v0.5.0 // indirect
	golang.org/x/tools v0.16.0 // indirect
)
EOF
    print_success "go.mod 已创建"
}

# ===== 创建构建脚本 =====
create_build_script() {
    print_step "创建构建脚本"
    
    cat > tun2socks/build.sh << 'EOFBUILD'
#!/bin/bash
set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}  AppProxy ECH Client Builder${NC}"
echo -e "${YELLOW}========================================${NC}"

SCRIPT_DIR=$(dirname "$0")
cd "$SCRIPT_DIR"

cd engine

if ! command -v go &> /dev/null; then
    echo -e "${RED}错误: 未找到Go环境${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Go版本: $(go version)${NC}"

echo -e "\n${YELLOW}[1/5] 初始化Go模块...${NC}"
go mod tidy

echo -e "\n${YELLOW}[2/5] 安装gomobile...${NC}"
go install golang.org/x/mobile/cmd/gomobile@latest

echo -e "\n${YELLOW}[3/5] 初始化gomobile...${NC}"
gomobile init

echo -e "\n${YELLOW}[4/5] 清理旧文件...${NC}"
rm -f ../../android/app/libs/proxyclient.aar
rm -f ../../android/app/libs/proxyclient-sources.jar

echo -e "\n${YELLOW}[5/5] 构建Android AAR...${NC}"
gomobile bind \
    -o ../../android/app/libs/proxyclient.aar \
    -target android \
    -androidapi 21 \
    -javapkg com.appproxy.client \
    -v \
    .

echo ""
if [ -f "../../android/app/libs/proxyclient.aar" ]; then
    FILE_SIZE=$(ls -lh ../../android/app/libs/proxyclient.aar | awk '{print $5}')
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}✅ 构建成功!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo "文件: android/app/libs/proxyclient.aar"
    echo "大小: $FILE_SIZE"
else
    echo -e "${RED}========================================${NC}"
    echo -e "${RED}❌ 构建失败${NC}"
    echo -e "${RED}========================================${NC}"
    exit 1
fi
EOFBUILD
    
    chmod +x tun2socks/build.sh
    print_success "build.sh 已创建"
}

# ===== 创建Flutter服务 =====
create_flutter_service() {
    print_step "创建Flutter服务层"
    
    cat > lib/services/proxy_manager.dart << 'EOF'
import 'package:flutter/services.dart';

class ProxyManager {
  static const platform = MethodChannel('com.appproxy.ech/proxy');
  
  /// 启动代理
  static Future<bool> startProxy({
    required String serverAddr,
    String serverIP = '',
    String token = '',
    String listenAddr = '127.0.0.1:1080',
  }) async {
    try {
      final bool result = await platform.invokeMethod('startProxy', {
        'serverAddr': serverAddr,
        'serverIP': serverIP,
        'token': token,
        'listenAddr': listenAddr,
      });
      return result;
    } on PlatformException catch (e) {
      print('启动代理失败: ${e.message}');
      return false;
    }
  }
  
  /// 停止代理
  static Future<bool> stopProxy() async {
    try {
      final bool result = await platform.invokeMethod('stopProxy');
      return result;
    } on PlatformException catch (e) {
      print('停止代理失败: ${e.message}');
      return false;
    }
  }
  
  /// 检查运行状态
  static Future<bool> isRunning() async {
    try {
      final bool result = await platform.invokeMethod('isRunning');
      return result;
    } on PlatformException catch (e) {
      print('检查状态失败: ${e.message}');
      return false;
    }
  }
  
  /// 测试连接
  static Future<bool> testConnection() async {
    try {
      final bool result = await platform.invokeMethod('testConnection');
      return result;
    } on PlatformException catch (e) {
      print('测试连接失败: ${e.message}');
      return false;
    }
  }
}
EOF
    
    print_success "proxy_manager.dart 已创建"
}

# ===== 创建GitHub Actions配置 =====
create_github_actions() {
    print_step "创建GitHub Actions配置"
    
    cat > .github/workflows/build.yml << 'EOFGH'
name: Build AppProxy ECH

on:
  push:
    branches: [ main, iyue ]
    tags: [ 'v*' ]
  pull_request:
    branches: [ main, iyue ]
  workflow_dispatch:

env:
  FLUTTER_VERSION: '3.19.0'
  GO_VERSION: '1.21'
  JAVA_VERSION: '17'

jobs:
  build:
    runs-on: ubuntu-latest
    
    steps:
    - name: 📥 Checkout代码
      uses: actions/checkout@v4
      with:
        ref: iyue
    
    - name: 🔧 设置Java环境
      uses: actions/setup-java@v4
      with:
        distribution: 'zulu'
        java-version: ${{ env.JAVA_VERSION }}
        cache: 'gradle'
    
    - name: 🐹 设置Go环境
      uses: actions/setup-go@v5
      with:
        go-version: ${{ env.GO_VERSION }}
        cache: true
        cache-dependency-path: tun2socks/engine/go.sum
    
    - name: 🎯 设置Flutter环境
      uses: subosito/flutter-action@v2
      with:
        flutter-version: ${{ env.FLUTTER_VERSION }}
        channel: 'stable'
        cache: true
    
    - name: 📦 安装gomobile
      run: |
        go install golang.org/x/mobile/cmd/gomobile@latest
        gomobile init
        echo "$HOME/go/bin" >> $GITHUB_PATH
    
    - name: 🔨 构建Go代理引擎
      run: |
        cd tun2socks/engine
        go mod tidy
        cd ..
        chmod +x build.sh
        ./build.sh
    
    - name: ✅ 验证AAR生成
      run: |
        if [ ! -f "android/app/libs/proxyclient.aar" ]; then
          echo "❌ AAR文件未生成"
          exit 1
        fi
        echo "✅ AAR文件已生成"
        ls -lh android/app/libs/proxyclient.aar
    
    - name: 📱 获取Flutter依赖
      run: flutter pub get
    
    - name: 🏗️ 构建APK (Debug)
      if: github.ref != 'refs/heads/main' && !startsWith(github.ref, 'refs/tags/')
      run: |
        flutter build apk --debug
        mv build/app/outputs/flutter-apk/app-debug.apk \
           build/app/outputs/flutter-apk/appproxy-ech-debug.apk
    
    - name: 🏗️ 构建APK (Release)
      if: github.ref == 'refs/heads/main' || startsWith(github.ref, 'refs/tags/')
      run: |
        flutter build apk --release
        mv build/app/outputs/flutter-apk/app-release.apk \
           build/app/outputs/flutter-apk/appproxy-ech-release.apk
    
    - name: 📊 生成版本信息
      run: |
        echo "Build Date: $(date)" > build_info.txt
        echo "Git Commit: ${{ github.sha }}" >> build_info.txt
        echo "Git Branch: ${{ github.ref_name }}" >> build_info.txt
        echo "Flutter Version: $(flutter --version | head -n 1)" >> build_info.txt
        echo "Go Version: $(go version)" >> build_info.txt
    
    - name: 📤 上传Debug APK
      if: github.ref != 'refs/heads/main' && !startsWith(github.ref, 'refs/tags/')
      uses: actions/upload-artifact@v4
      with:
        name: appproxy-ech-debug
        path: |
          build/app/outputs/flutter-apk/appproxy-ech-debug.apk
          build_info.txt
        retention-days: 7
    
    - name: 📤 上传Release APK
      if: github.ref == 'refs/heads/main' || startsWith(github.ref, 'refs/tags/')
      uses: actions/upload-artifact@v4
      with:
        name: appproxy-ech-release
        path: |
          build/app/outputs/flutter-apk/appproxy-ech-release.apk
          build_info.txt
        retention-days: 30

  release:
    needs: build
    runs-on: ubuntu-latest
    if: startsWith(github.ref, 'refs/tags/')
    
    steps:
    - name: 📥 下载构建产物
      uses: actions/download-artifact@v4
      with:
        name: appproxy-ech-release
        path: ./release
    
    - name: 🎉 创建GitHub Release
      uses: softprops/action-gh-release@v1
      with:
        files: |
          ./release/appproxy-ech-release.apk
          ./release/build_info.txt
        body: |
          ## AppProxy ECH客户端 ${{ github.ref_name }}
          
          ### 新功能
          - ✅ ECH (Encrypted Client Hello) 支持
          - ✅ WebSocket持久连接
          - ✅ SOCKS5 和 HTTP代理
          - ✅ DNS over HTTPS
          
          ### 下载
          `appproxy-ech-release.apk` - 通用版本
        draft: false
        prerelease: false
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
EOFGH
    
    print_success "GitHub Actions配置已创建"
}

# ===== 创建发布脚本 =====
create_release_script() {
    print_step "创建发布脚本"
    
    cat > release.sh << 'EOF'
#!/bin/bash
# 快速发布脚本

set -e

if [ -z "$1" ]; then
    echo "用法: ./release.sh <version>"
    echo "示例: ./release.sh 1.0.0"
    exit 1
fi

VERSION=$1
TAG="v${VERSION}"

echo "准备发布版本: $TAG"

# 检查是否有未提交的更改
if [[ -n $(git status -s) ]]; then
    echo "错误: 有未提交的更改"
    exit 1
fi

# 更新版本号
if [ -f "pubspec.yaml" ]; then
    sed -i.bak "s/^version: .*/version: $VERSION+1/" pubspec.yaml
    rm pubspec.yaml.bak
    git add pubspec.yaml
    git commit -m "Bump version to $VERSION"
fi

# 创建tag
git tag -a "$TAG" -m "Release $TAG"

# 推送
git push origin iyue
git push origin "$TAG"

echo "✅ 版本 $TAG 已发布"
echo "GitHub Actions将自动构建并创建Release"
EOF
    
    chmod +x release.sh
    print_success "release.sh 已创建"
}

# ===== 创建README =====
create_readme() {
    print_step "创建README"
    
    cat > README.md << 'EOF'
# AppProxy ECH客户端

基于ECH (Encrypted Client Hello) 和 WebSocket的现代化代理客户端。

## 特性

- ✅ ECH加密支持（TLS 1.3）
- ✅ WebSocket持久连接
- ✅ SOCKS5和HTTP代理协议
- ✅ DNS over HTTPS
- ✅ 自动ECH配置获取
- ✅ Flutter跨平台UI

## 快速开始

### 1. 添加源文件

将以下文件复制到对应目录：

**Go源文件** (`tun2socks/engine/`)
- `proxyclient.go`
- `android.go`

**Android文件** (`android/app/src/main/kotlin/com/appproxy/ech/`)
- `ProxyService.kt`
- `MainActivity.kt`

**Flutter文件** (`lib/pages/`)
- `proxy_page.dart`

### 2. 构建

```bash
# 构建Go引擎
cd tun2socks
./build.sh
cd ..

# 构建APK
flutter pub get
flutter build apk --release
```

### 3. 发布

```bash
# 创建release
./release.sh 1.0.0
```

## 配置

- **服务器地址**: `example.workers.dev:443`
- **监听地址**: `127.0.0.1:1080`

## 许可证

MIT License
EOF
    
    print_success "README.md 已创建"
}

# ===== 显示需要手动添加的文件 =====
show_manual_steps() {
    print_step "需要手动添加的文件"
    
    echo -e "${YELLOW}${BOLD}请将以下文件添加到项目中:${NC}\n"
    
    echo -e "${CYAN}Go源文件:${NC}"
    echo -e "  📁 tun2socks/engine/"
    echo -e "     ├── proxyclient.go   ${RED}(必需)${NC}"
    echo -e "     └── android.go       ${RED}(必需)${NC}"
    echo ""
    
    echo -e "${CYAN}Android Kotlin文件:${NC}"
    echo -e "  📁 android/app/src/main/kotlin/com/appproxy/ech/"
    echo -e "     ├── ProxyService.kt  ${RED}(必需)${NC}"
    echo -e "     └── MainActivity.kt  ${RED}(必需)${NC}"
    echo ""
    
    echo -e "${CYAN}Flutter页面:${NC}"
    echo -e "  📁 lib/pages/"
    echo -e "     └── proxy_page.dart  ${RED}(必需)${NC}"
    echo ""
    
    echo -e "${CYAN}Android配置:${NC}"
    echo -e "  📝 android/app/build.gradle       ${YELLOW}(需要修改)${NC}"
    echo -e "  📝 android/app/AndroidManifest.xml ${YELLOW}(需要修改)${NC}"
    echo ""
    
    print_info "详细代码请参考之前提供的artifacts"
}

# ===== 初始化Go模块 =====
init_go_module() {
    print_step "初始化Go模块"
    
    if [ ! -f "tun2socks/engine/proxyclient.go" ] || [ ! -f "tun2socks/engine/android.go" ]; then
        print_warning "Go源文件缺失，跳过初始化"
        return 0
    fi
    
    cd tun2socks/engine
    
    print_info "正在下载Go依赖..."
    go mod tidy
    
    print_success "Go模块初始化完成"
    
    cd ../..
}

# ===== 构建Go引擎 =====
build_go_engine() {
    print_step "构建Go代理引擎"
    
    if [ ! -f "tun2socks/engine/proxyclient.go" ] || [ ! -f "tun2socks/engine/android.go" ]; then
        print_error "Go源文件缺失，无法构建"
        print_info "请先添加 proxyclient.go 和 android.go"
        return 1
    fi
    
    print_info "开始构建AAR库 (可能需要几分钟)..."
    cd tun2socks
    ./build.sh
    cd ..
    
    if [ -f "android/app/libs/proxyclient.aar" ]; then
        print_success "AAR库构建成功"
        return 0
    else
        print_error "AAR库构建失败"
        return 1
    fi
}

# ===== 配置Flutter =====
setup_flutter() {
    print_step "配置Flutter项目"
    
    print_info "获取Flutter依赖..."
    flutter pub get
    print_success "Flutter依赖已安装"
}

# ===== 构建APK =====
build_apk() {
    print_step "构建Android APK"
    
    local build_type="${1:-debug}"
    
    print_info "构建类型: $build_type"
    
    if [ "$build_type" == "release" ]; then
        flutter build apk --release
    else
        flutter build apk --debug
    fi
    
    print_success "APK构建完成"
}

# ===== 显示完成信息 =====
show_completion() {
    print_banner
    echo -e "${GREEN}${BOLD}🎉 项目配置完成!${NC}\n"
    
    echo -e "${CYAN}${BOLD}项目目录:${NC}"
    echo -e "  📁 $(pwd)\n"
    
    echo -e "${CYAN}${BOLD}下一步操作:${NC}"
    echo -e "  1️⃣  添加必需的源文件 (见上方列表)"
    echo -e "  2️⃣  构建Go引擎: ${YELLOW}cd tun2socks && ./build.sh && cd ..${NC}"
    echo -e "  3️⃣  构建应用: ${YELLOW}flutter pub get && flutter build apk${NC}"
    echo -e "  4️⃣  推送到GitHub: ${YELLOW}git push origin iyue${NC}\n"
    
    echo -e "${CYAN}${BOLD}有用的命令:${NC}"
    echo -e "  • 测试运行: ${YELLOW}flutter run${NC}"
    echo -e "  • 查看日志: ${YELLOW}adb logcat | grep ProxyService${NC}"
    echo -e "  • 发布版本: ${YELLOW}./release.sh 1.0.0${NC}\n"
    
    echo -e "${CYAN}${BOLD}文档:${NC}"
    echo -e "  📚 README.md - 项目说明"
    echo -e "  🔧 tun2socks/build.sh - 构建脚本"
    echo -e "  🤖 .github/workflows/build.yml - CI/CD配置\n"
}

# ===== 交互菜单 =====
interactive_menu() {
    while true; do
        echo -e "\n${CYAN}${BOLD}请选择操作:${NC}"
        echo -e "  ${GREEN}1)${NC} 快速设置 (推荐新用户)"
        echo -e "  ${GREEN}2)${NC} 仅创建项目结构"
        echo -e "  ${GREEN}3)${NC} 构建Go引擎"
        echo -e "  ${GREEN}4)${NC} 构建Debug APK"
        echo -e "  ${GREEN}5)${NC} 构建Release APK"
        echo -e "  ${GREEN}6)${NC} 查看需要添加的文件"
        echo -e "  ${GREEN}7)${NC} 完整流程 (自动化)"
        echo -e "  ${RED}0)${NC} 退出"
        echo ""
        read -p "$(echo -e ${YELLOW}输入选项 [0-7]: ${NC})" choice
        
        case $choice in
            1)
                check_environment
                setup_repository
                create_structure
                create_go_module
                create_build_script
                create_flutter_service
                create_github_actions
                create_release_script
                create_readme
                show_manual_steps
                show_completion
                ;;
            2)
                create_structure
                create_go_module
                create_build_script
                print_success "项目结构创建完成"
                ;;
            3)
                init_go_module && build_go_engine
                ;;
            4)
                setup_flutter
                build_apk debug
                ;;
            5)
                setup_flutter
                build_apk release
                ;;
            6)
                show_manual_steps
                ;;
            7)
                check_environment
                setup_repository
                create_structure
                create_go_module
                create_build_script
                create_flutter_service
                create_github_actions
                create_release_script
                create_readme
                show_manual_steps
                init_go_module
                if build_go_engine; then
                    setup_flutter
                    build_apk debug
                fi
                show_completion
                ;;
            0)
                echo -e "\n${GREEN}再见!${NC}\n"
                exit 0
                ;;
            *)
                print_error "无效选项"
                ;;
        esac
    done
}

# ===== 主程序 =====
main() {
    print_banner
    
    # 检查命令行参数
    if [ $# -eq 0 ]; then
        interactive_menu
    else
        case $1 in
            --auto)
                check_environment
                setup_repository
                create_structure
                create_go_module
                create_build_script
                create_flutter_service
                create_github_actions
                create_release_script
                create_readme
                show_manual_steps
                show_completion
                ;;
            --full)
                check_environment
                setup_repository
                create_structure
                create_go_module
                create_build_script
                create_flutter_service
                create_github_actions
                create_release_script
                create_readme
                init_go_module
                if build_go_engine; then
                    setup_flutter
                    build_apk debug
                fi
                show_completion
                ;;
            --help|-h)
                echo "AppProxy ECH客户端一键部署脚本"
                echo ""
                echo "使用方法:"
                echo "  $0           - 交互式菜单"
                echo "  $0 --auto    - 自动设置项目结构"
                echo "  $0 --full    - 完整自动化流程"
                echo "  $0 --help    - 显示帮助"
                echo ""
                echo "示例:"
                echo "  bash <(curl -s https://你的仓库/one_click_deploy.sh)"
                ;;
            *)
                print_error "未知选项: $1"
                echo "使用 --help 查看帮助"
                exit 1
                ;;
        esac
    fi
}

# 运行主程序
main "$@"
