# 令牌管理 API 使用说明

## 接口列表

### 1. 添加令牌（Session/Access Token 鉴权）
- **接口**: `POST /api/token/`
- **鉴权**: 需要 Session 或 Access Token + `New-Api-User` header

### 2. 编辑令牌（Session/Access Token 鉴权）
- **接口**: `PUT /api/token/`
- **鉴权**: 需要 Session 或 Access Token + `New-Api-User` header

### 3. 统一添加/编辑令牌接口（Token 鉴权）
- **接口**: `POST /api/api/token/` 或 `PUT /api/api/token/`
- **鉴权**: 使用 `Authorization: Bearer sk-xxx` header
- **说明**: 如果请求中包含 `id` 字段，则为编辑模式；否则为添加模式

---

## 参数说明

### 基础参数

| 参数名 | 类型 | 必填 | 说明 | 示例 |
|--------|------|------|------|------|
| `id` | int | 编辑时必填 | 令牌ID（仅编辑时使用） | `123` |
| `name` | string | 是 | 令牌名称，最长50字符 | `"我的API令牌"` |
| `expired_time` | int64 | 否 | 过期时间（Unix时间戳），`-1` 表示永不过期 | `1735689600` 或 `-1` |
| `remain_quota` | int | 否 | 剩余额度（单位：quota） | `1000000` |
| `unlimited_quota` | bool | 否 | 是否无限额度，默认 `false` | `true` 或 `false` |
| `model_limits_enabled` | bool | 否 | 是否启用模型限制，默认 `false` | `true` 或 `false` |
| `model_limits` | string | 否 | 模型限制列表，多个模型用逗号分隔 | `"gpt-4,gpt-3.5-turbo"` |
| `allow_ips` | string | 否 | 允许访问的IP地址，多个IP用换行符或逗号分隔 | `"192.168.1.1\n192.168.1.2"` 或 `"192.168.1.1,192.168.1.2"` |
| `group` | string | 否 | 分组名称，空字符串表示使用用户默认分组 | `"auto"` 或 `"default"` |
| `cross_group_retry` | bool | 否 | 跨分组重试（仅 `auto` 分组有效），默认 `false` | `true` 或 `false` |
| `status` | int | 仅编辑 | 令牌状态：`1`=启用，`2`=禁用，`3`=过期，`4`=用尽 | `1` |

### 状态值说明

- `1` - 启用（TokenStatusEnabled）
- `2` - 禁用（TokenStatusDisabled）
- `3` - 过期（TokenStatusExpired）
- `4` - 用尽（TokenStatusExhausted）

---

## 使用示例

### 示例 1: 添加基础令牌（Session 鉴权）

```bash
curl -X POST http://your-domain/api/token/ \
  -H "Content-Type: application/json" \
  -H "New-Api-User: 1" \
  -b "session_cookie=your_session" \
  -d '{
    "name": "我的第一个令牌",
    "remain_quota": 1000000,
    "expired_time": -1,
    "unlimited_quota": false
  }'
```

**请求体（JSON）**:
```json
{
  "name": "我的第一个令牌",
  "remain_quota": 1000000,
  "expired_time": -1,
  "unlimited_quota": false
}
```

**响应**:
```json
{
  "success": true,
  "message": ""
}
```

---

### 示例 2: 添加无限额度令牌（Token 鉴权）

```bash
curl -X POST http://your-domain/api/api/token/ \
  -H "Authorization: Bearer sk-your-token-key" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "无限额度令牌",
    "unlimited_quota": true,
    "expired_time": -1
  }'
```

**请求体（JSON）**:
```json
{
  "name": "无限额度令牌",
  "unlimited_quota": true,
  "expired_time": -1
}
```

**响应**:
```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 123,
    "user_id": 1,
    "key": "sk-xxxxxxxxxxxx",
    "name": "无限额度令牌",
    "remain_quota": 0,
    "unlimited_quota": true,
    "expired_time": -1,
    "model_limits_enabled": false,
    "model_limits": "",
    "allow_ips": null,
    "group": "",
    "cross_group_retry": false,
    "status": 1
  }
}
```

---

### 示例 3: 添加带过期时间的令牌

```bash
curl -X POST http://your-domain/api/api/token/ \
  -H "Authorization: Bearer sk-your-token-key" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "临时令牌",
    "remain_quota": 500000,
    "expired_time": 1735689600,
    "unlimited_quota": false
  }'
```

**请求体（JSON）**:
```json
{
  "name": "临时令牌",
  "remain_quota": 500000,
  "expired_time": 1735689600,
  "unlimited_quota": false
}
```

**说明**: `expired_time` 是 Unix 时间戳（秒）。例如 `1735689600` 表示 2025-01-01 00:00:00 UTC。

---

### 示例 4: 添加带模型限制的令牌

```bash
curl -X POST http://your-domain/api/api/token/ \
  -H "Authorization: Bearer sk-your-token-key" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "GPT-4专用令牌",
    "remain_quota": 2000000,
    "expired_time": -1,
    "unlimited_quota": false,
    "model_limits_enabled": true,
    "model_limits": "gpt-4,gpt-4-turbo,gpt-4-32k"
  }'
```

**请求体（JSON）**:
```json
{
  "name": "GPT-4专用令牌",
  "remain_quota": 2000000,
  "expired_time": -1,
  "unlimited_quota": false,
  "model_limits_enabled": true,
  "model_limits": "gpt-4,gpt-4-turbo,gpt-4-32k"
}
```

**说明**: 
- `model_limits_enabled` 必须为 `true` 才能启用模型限制
- `model_limits` 是逗号分隔的模型名称列表

---

### 示例 5: 添加带IP限制的令牌

```bash
curl -X POST http://your-domain/api/api/token/ \
  -H "Authorization: Bearer sk-your-token-key" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "IP限制令牌",
    "remain_quota": 1000000,
    "expired_time": -1,
    "unlimited_quota": false,
    "allow_ips": "192.168.1.100\n10.0.0.50\n172.16.0.1"
  }'
```

**请求体（JSON）**:
```json
{
  "name": "IP限制令牌",
  "remain_quota": 1000000,
  "expired_time": -1,
  "unlimited_quota": false,
  "allow_ips": "192.168.1.100\n10.0.0.50\n172.16.0.1"
}
```

**说明**: 
- `allow_ips` 支持换行符（`\n`）或逗号（`,`）分隔多个IP
- 支持单个IP地址或CIDR格式（如 `192.168.1.0/24`）

---

### 示例 6: 添加指定分组的令牌

```bash
curl -X POST http://your-domain/api/api/token/ \
  -H "Authorization: Bearer sk-your-token-key" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Auto分组令牌",
    "remain_quota": 1000000,
    "expired_time": -1,
    "unlimited_quota": false,
    "group": "auto",
    "cross_group_retry": true
  }'
```

**请求体（JSON）**:
```json
{
  "name": "Auto分组令牌",
  "remain_quota": 1000000,
  "expired_time": -1,
  "unlimited_quota": false,
  "group": "auto",
  "cross_group_retry": true
}
```

**说明**: 
- `group` 为空字符串时使用用户默认分组
- `cross_group_retry` 仅在 `group` 为 `"auto"` 时有效

---

### 示例 7: 添加完整配置的令牌

```bash
curl -X POST http://your-domain/api/api/token/ \
  -H "Authorization: Bearer sk-your-token-key" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "完整配置令牌",
    "remain_quota": 5000000,
    "expired_time": 1735689600,
    "unlimited_quota": false,
    "model_limits_enabled": true,
    "model_limits": "gpt-4,claude-3-opus,gemini-pro",
    "allow_ips": "192.168.1.0/24,10.0.0.0/8",
    "group": "premium",
    "cross_group_retry": false
  }'
```

**请求体（JSON）**:
```json
{
  "name": "完整配置令牌",
  "remain_quota": 5000000,
  "expired_time": 1735689600,
  "unlimited_quota": false,
  "model_limits_enabled": true,
  "model_limits": "gpt-4,claude-3-opus,gemini-pro",
  "allow_ips": "192.168.1.0/24,10.0.0.0/8",
  "group": "premium",
  "cross_group_retry": false
}
```

---

### 示例 8: 编辑令牌（更新名称和额度）

```bash
curl -X PUT http://your-domain/api/api/token/ \
  -H "Authorization: Bearer sk-your-token-key" \
  -H "Content-Type: application/json" \
  -d '{
    "id": 123,
    "name": "更新后的令牌名",
    "remain_quota": 10000000,
    "expired_time": -1,
    "unlimited_quota": false
  }'
```

**请求体（JSON）**:
```json
{
  "id": 123,
  "name": "更新后的令牌名",
  "remain_quota": 10000000,
  "expired_time": -1,
  "unlimited_quota": false
}
```

**响应**:
```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 123,
    "user_id": 1,
    "key": "sk-xxxxxxxxxxxx",
    "name": "更新后的令牌名",
    "remain_quota": 10000000,
    "unlimited_quota": false,
    "expired_time": -1,
    "model_limits_enabled": false,
    "model_limits": "",
    "allow_ips": null,
    "group": "",
    "cross_group_retry": false,
    "status": 1
  }
}
```

---

### 示例 9: 仅更新令牌状态

```bash
curl -X PUT http://your-domain/api/api/token/?status_only=1 \
  -H "Authorization: Bearer sk-your-token-key" \
  -H "Content-Type: application/json" \
  -d '{
    "id": 123,
    "status": 2
  }'
```

**请求体（JSON）**:
```json
{
  "id": 123,
  "status": 2
}
```

**说明**: 
- 使用 `?status_only=1` 查询参数时，只更新 `status` 字段
- 状态值：`1`=启用，`2`=禁用，`3`=过期，`4`=用尽

---

### 示例 10: 编辑令牌启用模型限制

```bash
curl -X PUT http://your-domain/api/api/token/ \
  -H "Authorization: Bearer sk-your-token-key" \
  -H "Content-Type: application/json" \
  -d '{
    "id": 123,
    "name": "GPT-4专用令牌",
    "model_limits_enabled": true,
    "model_limits": "gpt-4,gpt-4-turbo"
  }'
```

**请求体（JSON）**:
```json
{
  "id": 123,
  "name": "GPT-4专用令牌",
  "model_limits_enabled": true,
  "model_limits": "gpt-4,gpt-4-turbo"
}
```

---

### 示例 11: 编辑令牌更新IP限制

```bash
curl -X PUT http://your-domain/api/api/token/ \
  -H "Authorization: Bearer sk-your-token-key" \
  -H "Content-Type: application/json" \
  -d '{
    "id": 123,
    "allow_ips": "192.168.1.100,192.168.1.101,10.0.0.50"
  }'
```

**请求体（JSON）**:
```json
{
  "id": 123,
  "allow_ips": "192.168.1.100,192.168.1.101,10.0.0.50"
}
```

**说明**: 要清除IP限制，可以传入空字符串 `""` 或 `null`

---

### 示例 12: 编辑令牌设置为无限额度

```bash
curl -X PUT http://your-domain/api/api/token/ \
  -H "Authorization: Bearer sk-your-token-key" \
  -H "Content-Type: application/json" \
  -d '{
    "id": 123,
    "unlimited_quota": true,
    "remain_quota": 0
  }'
```

**请求体（JSON）**:
```json
{
  "id": 123,
  "unlimited_quota": true,
  "remain_quota": 0
}
```

---

## 注意事项

1. **令牌名称限制**: `name` 字段最长50字符
2. **过期时间**: 
   - `-1` 表示永不过期
   - 其他值为 Unix 时间戳（秒）
3. **额度设置**: 
   - 如果 `unlimited_quota` 为 `true`，`remain_quota` 会被忽略
   - 如果 `unlimited_quota` 为 `false`，必须设置 `remain_quota`
4. **模型限制**: 
   - `model_limits_enabled` 为 `true` 时，`model_limits` 才会生效
   - `model_limits` 为空字符串时，表示不限制任何模型
5. **IP限制**: 
   - 支持单个IP、多个IP（逗号或换行分隔）、CIDR格式
   - 空字符串或 `null` 表示不限制IP
6. **分组设置**: 
   - `group` 为空字符串时使用用户默认分组
   - `cross_group_retry` 仅在 `group` 为 `"auto"` 时有效
7. **编辑模式**: 
   - 必须提供 `id` 字段
   - 只更新提供的字段，未提供的字段保持原值
   - 使用 `?status_only=1` 查询参数时，只更新状态字段

---

## 错误响应示例

### 令牌名称过长
```json
{
  "success": false,
  "message": "令牌名称过长"
}
```

### 令牌不存在（编辑时）
```json
{
  "success": false,
  "message": "令牌不存在"
}
```

### 令牌已过期，无法启用
```json
{
  "success": false,
  "message": "令牌已过期，无法启用，请先修改令牌过期时间，或者设置为永不过期"
}
```

### 令牌额度已用尽，无法启用
```json
{
  "success": false,
  "message": "令牌可用额度已用尽，无法启用，请先修改令牌剩余额度，或者设置为无限额度"
}
```

---

## Python 示例

```python
import requests
import json

# 添加令牌
def add_token(token_key, name, remain_quota=1000000, expired_time=-1):
    url = "http://your-domain/api/api/token/"
    headers = {
        "Authorization": f"Bearer {token_key}",
        "Content-Type": "application/json"
    }
    data = {
        "name": name,
        "remain_quota": remain_quota,
        "expired_time": expired_time,
        "unlimited_quota": False
    }
    response = requests.post(url, headers=headers, json=data)
    return response.json()

# 编辑令牌
def update_token(token_key, token_id, **kwargs):
    url = "http://your-domain/api/api/token/"
    headers = {
        "Authorization": f"Bearer {token_key}",
        "Content-Type": "application/json"
    }
    data = {"id": token_id, **kwargs}
    response = requests.put(url, headers=headers, json=data)
    return response.json()

# 使用示例
result = add_token(
    token_key="sk-your-token",
    name="我的令牌",
    remain_quota=5000000,
    expired_time=-1
)
print(result)

result = update_token(
    token_key="sk-your-token",
    token_id=123,
    name="更新后的名称",
    remain_quota=10000000
)
print(result)
```

---

## JavaScript/Node.js 示例

```javascript
const axios = require('axios');

// 添加令牌
async function addToken(tokenKey, name, remainQuota = 1000000, expiredTime = -1) {
  const response = await axios.post(
    'http://your-domain/api/api/token/',
    {
      name: name,
      remain_quota: remainQuota,
      expired_time: expiredTime,
      unlimited_quota: false
    },
    {
      headers: {
        'Authorization': `Bearer ${tokenKey}`,
        'Content-Type': 'application/json'
      }
    }
  );
  return response.data;
}

// 编辑令牌
async function updateToken(tokenKey, tokenId, updates) {
  const response = await axios.put(
    'http://your-domain/api/api/token/',
    {
      id: tokenId,
      ...updates
    },
    {
      headers: {
        'Authorization': `Bearer ${tokenKey}`,
        'Content-Type': 'application/json'
      }
    }
  );
  return response.data;
}

// 使用示例
(async () => {
  const result = await addToken(
    'sk-your-token',
    '我的令牌',
    5000000,
    -1
  );
  console.log(result);

  const updateResult = await updateToken(
    'sk-your-token',
    123,
    {
      name: '更新后的名称',
      remain_quota: 10000000
    }
  );
  console.log(updateResult);
})();
```

