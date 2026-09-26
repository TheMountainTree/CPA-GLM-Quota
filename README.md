# CPA-GLM-Quota

> **CLIProxyAPI (CPA)** 专属的智谱 **GLM Coding Plan** 配额监控动态库插件。

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Platform](https://img.shields.io/badge/Platform-Linux%20(amd64%20%7C%20arm64)-FC6D26?style=flat&logo=linux)](https://github.com/TheMountainTree/CPA-GLM-Quota)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

---

## 📌 背景与特性

在 [CLIProxyAPI (CPA)](https://github.com/router-for-me/CLIProxyAPI) 中，智谱 GLM Coding Plan 通常以 API Key 形式通过 `openai-compatibility` 接入。由于缺少官方 OAuth 流程，CPA 自带的「配额管理」页面无法展示该套餐的额度消耗及刷新倒计时。

**CPA-GLM-Quota** 是一个专为解决此痛点开发的 CPA 动态库插件（`.so`），以只读旁路方式运行，不影响任何模型转发。

### ✨ 核心特性

- 📊 **双周期滑动监控**：实时展示 **5 小时滚动配额**、**自然周总配额**以及 **MCP 月度工具调用**消耗详情与剩余额度。
- ⏱️ **精准倒计时**：清晰显示下一次配额恢复的具体时间点及剩余小时/分钟倒计时。
- 🎨 **完美契合 CPAMC 官方设计系统**：
  - 统一的米灰质感、微浮雕圆角卡片、等宽数字与状态指示条；
  - 还原配额调度时间轴（甘特图风格）；
  - **自动跟随 CPAMC 主题换肤**：内嵌 DOM 监听，父级控制台切换【浅色 / 纯白 / 暗色】模式时毫秒级无缝同步。
- 🔄 **强兼容解析引擎**：同时兼容智谱新版 **`CREDIT_LIMIT`（积分）** 与旧版 **`TOKENS_LIMIT`**，无惧官方数据结构微调。
- ⚡ **高性能 & 零侵入**：
  - 仅注册 `ManagementAPI` 资源，绝不劫持或影响模型对话请求；
  - 内置 60 秒内存安全缓存，杜绝高频刷新触发智谱 API 风控。
- 🔌 **双模式交付**：既可在控制台内直接嵌入可视化卡片，也可通过 `?format=json` 获取纯 JSON 数据供脚本调用。

---

## 🚀 快速部署指南

### 第一步：获取 `glm-quota.so` 动态库

你可以直接从 [Releases](../../releases) 页面下载预编译的 Linux x86_64 动态库，或在本地自编译。

将 `glm-quota.so` 上传到 VPS 宿主机的插件目录（例如 `/root/cliproxy/plugins/`）：

```bash
mkdir -p /root/cliproxy/plugins
# 上传 glm-quota.so 到该目录
scp glm-quota.so user@your-vps:/root/cliproxy/plugins/
```

### 第二步：挂载插件目录至 Docker 容器

修改 `docker-compose.yml`，在 `volumes` 中添加只读挂载映射：

```yaml
version: "3.3"
services:
  cli-proxy-api:
    image: eceasy/cli-proxy-api:latest
    container_name: cli-proxy-api
    restart: always
    ports:
      - "127.0.0.1:8317:8317"
    volumes:
      - ./config.yaml:/CLIProxyAPI/config.yaml
      - ./auths:/root/.cli-proxy-api
      - ./plugins:/CLIProxyAPI/plugins:ro    # 👈 新增此行挂载
```

### 第三步：在 `config.yaml` 启用插件

在 `/root/cliproxy/config.yaml` 文件中追加插件配置项：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    glm-quota:
      enabled: true
      provider: bigmodel        # 国内版填写 bigmodel，国际版（z.ai）填写 zai
      api-key: "your-coding-plan-api-key"   # 你的 GLM Coding Plan API Key
```

### 第四步：重启并生效

```bash
cd /root/cliproxy
docker-compose up -d --force-recreate
```

查看容器日志，确认插件成功加载：
```bash
docker logs --tail 30 cli-proxy-api | grep -iE "pluginhost|glm-quota"
# 应输出：pluginhost: plugin loaded plugin_id=glm-quota ...
```

---

## 🖥️ 访问与使用

打开 CPAMC（CLI Proxy API 管理控制台），在左侧菜单栏底部的 **【插件】** 分类中，点击 **「GLM Quota」** 即可进入全新配额仪表盘。

### 接口与访问路由

| 模式 | 地址 | 说明 |
| :--- | :--- | :--- |
| **可视化仪表盘** | `/v0/resource/plugins/glm-quota/status` | CPAMC 风格交互界面，自动适配主题 |
| **JSON 数据接口** | `/v0/resource/plugins/glm-quota/status?format=json` | 适合自动化监控脚本或桌面小组件 |
| **管理 API 路由** | `/v0/management/glm-quota?format=json` | 带 Management Key 鉴权的管理接口 |

---

## 🛠️ 本地编译构建

本项目基于 Go C-Shared 动态库（CGO）构建。推荐使用 Docker 保证与生产环境 glibc 基线对齐：

```bash
# 使用 Docker 一键编译 Linux amd64 动态库
make build-docker

# 或者在具备 CGO 环境的 Linux 宿主机上直接编译：
make build
```

---

## 📄 开源许可证

本项目基于 [MIT 许可证](LICENSE) 分发。
