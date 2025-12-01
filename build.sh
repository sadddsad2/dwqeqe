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
