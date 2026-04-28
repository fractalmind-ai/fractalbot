# O7 KR2 本地验证清单（callback-first）

> Status: draft
> Updated: 2026-03-23 02:00 PDT
> Purpose: 把 KR2 的 callback-first 路线落到一套最小可执行的本地验证步骤，方便后续直接补运行证据。

## 1. 目标

在 **不安装/升级 OpenClaw core** 的前提下，基于当前 `fractalmind-ai/fractalbot` 运行时，完成如下最小验证：

1. wechat channel 能在当前 gateway 中被启用
2. callback listener 能真正启动
3. 本地 GET / POST 请求能打到 callback 路径
4. 留下启动命令、状态输出、请求响应作为 KR2 证据

## 2. 最小配置草案

可基于 `config.example.yaml` 的 `channels.wechat` 段落，准备一份本地临时配置：

```yaml
channels:
  wechat:
    enabled: true
    provider: "wecom"
    callbackListenAddr: "127.0.0.1:18810"
    callbackPath: "/wechat/callback"
    callbackToken: "local-test-token"
    callbackEncodingAESKey: ""
    corpId: ""
    corpSecret: ""
    agentId: ""
    defaultAgent: "main"
    allowedAgents:
      - "main"
    syncReplyTimeoutSeconds: 4
    asyncSendEnabled: true
    accessTokenCacheFile: "./workspace/wechat.token.json"
```

说明：
- 这里只是 **本地 listener / path / handler 验证配置**
- 不等于已接通真实官方账号
- 首轮验证不需要先填真实企业微信密钥

## 3. 推荐验证步骤

### Step 1: 启动 gateway

```bash
cd projects/fractalmind-ai/fractalbot
fractalbot --config ./config.yaml
```

或使用本地临时配置文件启动。

### Step 2: 检查状态输出

```bash
curl -s http://127.0.0.1:18789/status | jq '.'
```

理想证据：
- `wechat` 出现在 channels 列表中
- 状态为 running / initialized（或等价状态）

### Step 3: 验证 GET callback 握手路径

按当前 helper 逻辑，构造：

```bash
python3 - <<'PY2'
import hashlib
items = sorted(["local-test-token", "1", "2"])
print(hashlib.sha1("".join(items).encode()).hexdigest())
PY2
```

然后请求：

```bash
curl -i "http://127.0.0.1:18810/wechat/callback?signature=<sha1>&timestamp=1&nonce=2&echostr=hello"
```

理想证据：
- 返回 `200`
- body 直接回 `hello`

### Step 4: 验证 POST callback 路径

```bash
curl -i   -X POST "http://127.0.0.1:18810/wechat/callback?signature=<sha1>&timestamp=1&nonce=2"   -H 'Content-Type: application/xml'   --data '<xml><ToUserName><![CDATA[toUser]]></ToUserName><FromUserName><![CDATA[fromUser]]></FromUserName><CreateTime>1348831860</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[this is a test]]></Content><MsgId>1234567890123456</MsgId></xml>'
```

理想证据：
- 能得到当前 scaffold 的结构化响应
- 能看到 `protocol_message` / handler 路由相关字段

## 4. 本轮先不做什么

当前 checklist 明确**不包含**：

- 真实微信账号登录
- OpenClaw 安装/升级
- polling loop 实现
- iLink API 登录态验证

这些动作要么超出 callback-first 第一阶段目标，要么与当前用户约束冲突。

## 5. KR2 证据清单模板

后续正式执行时，建议至少保留：

1. 启动命令
2. 使用的本地配置片段
3. `/status` 输出
4. GET callback 响应
5. POST callback 响应
6. 必要日志摘录

只要这 6 类证据齐，就能把 KR2 从“路线分析”推进到“当前运行时内的最小运行证据”。
