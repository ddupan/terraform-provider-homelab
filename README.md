# terraform-provider-homelab

Homelab 平台自有 API 的 Terraform provider。所有后端都以调用方的 **SPIFFE 身份**认证：每次请求从本地
Workload API 取对应 audience 的 JWT-SVID，provider 不保存任何凭据。资源按领域命名（`homelab_<domain>_…`），
新后端以 provider 配置中独立的块接入。

| 资源 | 后端 | 说明 |
|---|---|---|
| `homelab_oidc_client` | iam-login（`sso.ddupan.top`） | 授权码 OIDC 客户端（confidential 或 public）；secret 只在创建/轮换时可得 |

## 用法

```hcl
terraform {
  required_providers {
    homelab = { source = "terraform.lab.ddupan.top/ddupan/homelab" }
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
- public 客户端（CLI、浏览器应用，如 Incus）：`public_client = true`，没有 secret（`secret` 为 null），必须保持
  `pkce_required = true`；创建后不能改类型（改动即替换）。`device_authorization = true` 另外开启 device 授权
  （RFC 8628）；`audiences` 给自己校验 `aud` 的资源加上额外 audience。需要 iam-login 支持这三个字段的版本。

  ```hcl
  resource "homelab_oidc_client" "incus" {
    id                   = "incus"
    name                 = "Incus"
    redirect_uris        = ["https://incus.lab.ddupan.top/oidc/callback"]
    scopes               = ["openid", "offline_access"]
    public_client        = true
    device_authorization = true
    audiences            = ["https://incus.lab.ddupan.top"]
  }
  ```
- `token_endpoint_auth_method`（RFC 7591）：confidential 客户端出示 secret 的方式，缺省 `client_secret_basic`。
  只给发不了 Basic 的应用设 `client_secret_post`（如 Artifact Keeper）；public 客户端固定 `none`。从配置里删掉
  不会改回，要改回就显式写 `client_secret_basic`。需要支持该字段的 iam-login 版本。
- 导入：`terraform import homelab_oidc_client.gitea gitea`。已有 secret 无法读回，导入后设置 `secret_rotation` 取得新 secret。

## 开发

```sh
go test ./...
go build -o terraform-provider-homelab
```

本地试用可在 `~/.terraformrc` 用 `dev_overrides` 指向构建产物。

## 发布

发布分两半，签名只在维护者本机完成：

1. 打 `vX.Y.Z` tag：`release` job 测试后执行 `scripts/release`，构建 linux/amd64、linux/arm64 的 zip 与
   `SHA256SUMS`，上传到静态 registry 的 bucket（homelab-infra `apps/seaweedfs` 的 `terraform-registry`）。
   此时 terraform 还找不到这个版本。已发布的版本拒绝再次上传（重建的 zip 哈希不同，会让已签名的版本失效）。
2. 本机执行 `scripts/publish X.Y.Z`：下载 zip 与 `SHA256SUMS` 并逐一校验，用维护者 YubiKey 上的签名子钥
   `8A887573EFE9762D`（主钥 `0166F47B5400ECC2`）签名（按提示触摸），再写签名、各平台 download 文档，最后更新
   versions 索引。上传凭据经本机 SPIFFE 开发身份从 OpenBao 读取。

签名子钥 **2027-04-07 到期**。延期后对每个已发布版本重新执行 `scripts/publish <version>`，刷新文档里嵌入的公钥。
