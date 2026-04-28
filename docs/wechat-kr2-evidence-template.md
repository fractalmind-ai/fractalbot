# O7 KR2 运行证据模板（callback-first）

> Status: draft
> Updated: 2026-03-23 03:00 PDT
> Purpose: 为下一轮 O7-KR2 正式执行准备统一证据模板，确保启动、状态、GET/POST callback 结果都能一次性归档。

## 1. 环境信息

- Date:
- Host:
- Working tree / branch:
- FractalBot config path:
- WeChat provider:

## 2. 启动命令

```bash
cd projects/fractalmind-ai/fractalbot
fractalbot --config <config-path>
```

### 记录
- Command:
- Exit / running status:
- Key log lines:

## 3. Gateway 状态证据

```bash
curl -s http://127.0.0.1:18789/status | jq '.'
```

### 记录
- Time:
- Whether `wechat` appears in channels:
- Channel state / telemetry:
- Raw output snippet:

## 4. GET callback 证据

### 签名计算

```bash
python3 - <<'PY2'
import hashlib
items = sorted(["<token>", "1", "2"])
print(hashlib.sha1("".join(items).encode()).hexdigest())
PY2
```

### 请求

```bash
curl -i "http://127.0.0.1:18810/wechat/callback?signature=<sha1>&timestamp=1&nonce=2&echostr=hello"
```

### 记录
- Request:
- Expected: HTTP 200 + `hello`
- Actual status:
- Actual body:

## 5. POST callback 证据

```bash
curl -i   -X POST "http://127.0.0.1:18810/wechat/callback?signature=<sha1>&timestamp=1&nonce=2"   -H 'Content-Type: application/xml'   --data '<xml><ToUserName><![CDATA[toUser]]></ToUserName><FromUserName><![CDATA[fromUser]]></FromUserName><CreateTime>1348831860</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[this is a test]]></Content><MsgId>1234567890123456</MsgId></xml>'
```

### 记录
- Request:
- Expected: scaffold returns structured response
- Actual status:
- Actual body:
- Whether `protocol_message` / `handler_*` fields are present:

## 6. 结论

- Did we prove listener startup?
- Did we prove `/status` visibility?
- Did we prove GET callback handshake?
- Did we prove POST callback parsing / routing?
- Remaining gap to real official capability:

## 7. 后续动作

- If all above pass: advance KR2 from “checklist” to “local runtime evidence exists”
- If any step fails: record exact error, retry count, and whether blocker is config / code / runtime
