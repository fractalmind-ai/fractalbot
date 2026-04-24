# O7 KR2 polling-first 本地验证清单

> Status: draft
> Updated: 2026-03-23 08:55 PDT
> Purpose: 为 KR2 的 polling-first 第一阶段准备本地验证步骤。

## 1. 目标

在不安装/升级 OpenClaw core 的前提下，验证：

1. 当前运行时能挂接 polling 型 wechat channel
2. 能读取 token/baseUrl/state
3. 能运行 poll loop 或等价模拟轮询
4. 能把 polling 消息映射进现有 handler
5. 能留下 state / cursor 更新证据

## 2. 本地验证步骤（第一版）

### Step 1
准备最小 polling 配置草案

### Step 2
确认 channel manager 能识别 polling 模式的 wechat channel

### Step 3
补一个最小 poll loop / mock updates 入口

### Step 4
记录一次消息进入 handler 的证据

### Step 5
记录 state file / cursor 更新证据

## 3. 当前不做

- callback GET/POST 证明
- 真实官方账号完整登录闭环
- 多账号与媒体能力
