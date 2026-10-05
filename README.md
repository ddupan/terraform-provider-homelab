# terraform-provider-homelab

Homelab 平台自有 API 的 Terraform provider。所有后端都以调用方的 **SPIFFE 身份**认证：每次请求从本地
Workload API 取对应 audience 的 JWT-SVID，provider 不保存任何凭据。资源按领域命名（`homelab_<domain>_…`），
新后端以 provider 配置中独立的块接入。

| 资源 | 后端 | 说明 |
|---|---|---|
| `homelab_oidc_client` | iam-login（`sso.ddupan.top`） | confidential 授权码 OIDC 客户端；secret 只在创建/轮换时可得 |

## 用法

```hcl
terraform {
  required_providers {
    homelab = { source = "git.ddupan.top/ddupan/homelab" }
  }
}

provider "homelab" {
  iam_login = { endpoint = "https://sso.ddupan.top" }
  # spiffe_endpoint_socket 默认取 SPIFFE_ENDPOINT_SOCKET
}

resource "homelab_oidc_client" "gitea" {
  id            = "gitea"
  name          = "Gitea"
  redirect_uris = ["https://git.ddupan.top/user/oauth2/sso/callback"]
  scopes        = ["openid", "profile", "email", "groups"]
  pkce_required = false # Gitea 1.27 不发 PKCE
}
```

- 调用方的 SPIFFE ID 须在 iam-login 的 `iam.clients.spiffe.admin-ids` 中，JWT-SVID audience 为 `iam-login`。
- `secret` 为 sensitive，会进入 state；把它写入 OpenBao（如 `vault_kv_secret_v2`）供应用读取。state 本身按敏感数据保管。
- 改 `secret_rotation` 的值即轮换 secret，旧值立即失效。
- 导入：`terraform import homelab_oidc_client.gitea gitea`。已有 secret 无法读回，导入后设置 `secret_rotation` 取得新 secret。

## 开发

```sh
go test ./...
go build -o terraform-provider-homelab
```

本地试用可在 `~/.terraformrc` 用 `dev_overrides` 指向构建产物；正式分发方式（filesystem mirror 或 Gitea
package registry）随首个 release 确定。
