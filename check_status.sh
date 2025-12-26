#!/bin/bash
# 采集状态检查脚本

cd "$(dirname "$0")"

echo "========== 采集程序状态检查 =========="
echo "时间: $(date)"
echo ""

# 检查程序是否在运行
if ps aux | grep -v grep | grep "go run" > /dev/null; then
    echo "✅ 采集程序正在运行中"
else
    echo "❌ 采集程序未运行"
fi
echo ""

# 检查生成的文件
echo "=== 文件状态 ==="
if [ -f "nodes.txt" ]; then
    NODE_COUNT=$(wc -l < nodes.txt | tr -d ' ')
    echo "✅ nodes.txt: $NODE_COUNT 个节点"
else
    echo "⏳ nodes.txt: 尚未生成"
fi

echo ""

# 显示最新日志
echo "=== 最新采集进度（最后10行）==="
if [ -f "collect_output.log" ]; then
    tail -10 collect_output.log | grep -E "(开始采集|采集完成|ghelper|JMS|共找到|解析出|可用节点|节点已保存)" || tail -5 collect_output.log
else
    echo "日志文件不存在"
fi
echo ""

# 显示采集阶段
echo "=== 当前采集阶段 ==="
if [ -f "collect_output.log" ]; then
    if grep -q "ghelper 节点采集完成" collect_output.log; then
        echo "✅ ghelper 采集已完成"
    elif grep -q "开始采集 ghelper" collect_output.log; then
        echo "🔄 ghelper 采集进行中"
    elif grep -q "JMS 节点采集完成" collect_output.log; then
        echo "✅ JMS 采集已完成，等待 ghelper 开始"
    elif grep -q "开始采集 JMS" collect_output.log; then
        echo "🔄 JMS 采集进行中"
    else
        echo "⏳ 等待采集开始"
    fi
fi

