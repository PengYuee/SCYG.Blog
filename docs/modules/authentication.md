# 登录与会话

## 范围与参与方

本模块说明作者如何登录、恢复浏览器会话、访问受保护功能，以及失效和退出后的行为。它涉及前端登录页、请求与路由边界，以及 Blog 的用户、JWT 和内容授权实现。访客浏览公开文章不需要作者账号；匿名 AI 搜索的独立身份由[公共 AI 搜索](public-ai-search.md)维护，不把匿名凭证当作者登录。

当前提供用户名/密码登录和短期访问令牌，没有注册、找回密码、修改密码、refresh、me 或服务端 logout 业务。`/admin` 仍是未开放的路由边界，不能因有名为 admin 的本地账号就宣称已有管理后台或角色系统。默认本地联调账号只在[根规则](../../AGENTS.md#本地开发与联调账号)维护，不作为生产部署凭据。

## 正常登录流程

1. 用户直接进入 `/login`，或访问作者地址时由路由守卫带原站内地址转到登录页。
2. 前端提交用户名和密码到 Blog 登录接口；重复点击期间只保持一次提交。REST 边界去掉用户名首尾空白，不去掉密码空白。
3. Blog 查询同名活动账号，核对 bcrypt 密码哈希。账号不存在、已停用或密码错误统一按凭据无效处理，不区分暴露原因；数据库或签发异常不伪装成密码错误。
4. 登录成功后 Blog 返回短期 Bearer 令牌及到期时间。前端用生成的响应 Schema 校验后保存会话，清理旧作者 AI 临时状态，再返回原受保护地址；未给出合法返回目标时进入新建文章页。
5. 返回目标必须是站内以 `/` 开头且不以 `//` 开头的路径，不把外部地址作为登录跳转目标。前端无论成功或失败都会清空密码输入；失败保留用户名并显示可判断的错误。

接口方法、字段和状态码以 [OpenAPI](../../backend/api/openapi.yaml) 的登录操作为准，不在本文维护第二份请求模型。

## 浏览器会话的生命周期

| 事件 | 当前行为与完成边界 |
| --- | --- |
| 登录成功 | 通过响应校验且未过期后，才保存到 localStorage 的 `scyg.auth.session` 并进入已认证状态；浏览器不解码 JWT 来判断角色或资源权限。 |
| 页面启动或刷新 | 读取存储并校验结构、到期时间；有效则恢复，否则丢弃畸形数据或转为已过期/匿名状态。恢复本地会话不等于向服务端刷新令牌。 |
| 使用会话 | 路由守卫、写入门禁及 token provider 读取同一控制器，读取时仍检查到期时间。只有显式选择作者认证的请求才携带作者 Bearer。 |
| 作者请求返回 `401` | 失效与该请求 token 仍匹配的作者会话，避免旧请求清掉后来登录的新会话；后续作者导航要求重新登录。 |
| 本地退出 | 删除浏览器会话并回到匿名状态，不调用不存在的 logout 接口，也不撤销服务端已经签发的令牌。 |
| 跨 tab 登录变更 | localStorage 会话变更或 `clear()` 的 `key=null` 事件触发恢复，清掉旧作者 AI 临时状态；同 tab 的登录、退出由控制器直接同步。 |

登录令牌的持久存储与 AI 草稿的 tab 存储用途不同。作者 AI 的表单、游标、交互草稿和未知操作按主体保存在 sessionStorage；失效、退出或换账号时停止旧作者请求/订阅并清理该分区。匿名分区独立，匿名 `401` 不使作者登出。具体任务恢复和未知操作语义见 [AI 任务与作者辅助](ai-assistance.md)。

## 后端认证与业务授权不是同一件事

作者 HTTP 请求的 Bearer 必须通过签名算法、签名、issuer、到期时间和 subject 校验。当前令牌使用 HS256，身份 subject 为合法用户标识。受保护路由缺少或收到无效凭据时返回 `401`；公开文章等路由不因浏览器持有作者会话而自动切换成管理读取。

默认生产装配的内容 `ContextAuthorizer` 目前只检查上下文中是否存在受信作者 Principal，不按 action/resource 构造角色矩阵，也没有文章创建者级隔离。因此不能把“作者工作区”解释为“只能管理自己创建的文章”。图片归属校验、AI Run owner 与内容范围另有各自的业务规则，见[文章图片](article-images.md)、[AI 任务](ai-assistance.md)及[公共搜索](public-ai-search.md)。缺省或无效授权协作者沿拒绝策略处理，不能退回任意放行。

内部 BlogContent 的 author 调用根据可信 `user_id` 重新查询活动账号后建立业务身份；它不是浏览器 JWT 验证的替代接口，内部 gRPC 不应暴露给浏览器。public 调用不在作者账号表中伪造用户。

登录会检查账号活动状态，但当前普通 HTTP JWT 验证不逐请求重新查询用户活动状态，也没有令牌撤销列表。停用账号会阻止再次登录，不能据此承诺已签发 token 立即失效；本地退出也不能保证其他客户端的 token 被撤回。这是现有能力边界，不在本模块描述为已经实现的账号治理系统。

## 失败与拒绝

- 输入不符合登录传输合同：按 API 输入错误处理，前端不能把它当已登录；账号不存在、停用或密码错误是统一的 `401`。
- 本地会话畸形或过期：不继续向作者接口投递旧 token，恢复到可重新登录的状态。
- 请求身份有效但业务拒绝：由具体模块返回其权限或状态错误；前端已认证不意味着业务一定成功。
- Run 不存在、非 owner 或内容范围不允许：由任务模块统一呈现 `404`，不是要求前端自行解码凭证区分原因。
- 网络结果不明：登录页保留可重新提交的用户名，不伪造成功或自动续期；AI mutation 的未知结果不能照搬普通登录重试，按任务模块的幂等规则处理。

## 数据归属与维护入口

用户标识、账号活动状态与密码哈希归 Blog identity；token 的密钥、issuer 和有效期由 Blog 运行配置维护，不复制到本文或前端。浏览器存储仅是客户端会话状态，最终请求权限仍由服务端执行。

| 需要核对的行为 | 源码与验证入口 |
| --- | --- |
| 活动账号和密码校验 | [LoginService](../../backend/internal/modules/identity/auth/login_service.go)、[用户仓储](../../backend/internal/modules/identity/user/user_repository.go)、[用户模型](../../backend/internal/modules/identity/user/user.go) |
| token 签发与验证 | [TokenService](../../backend/internal/modules/identity/auth/jwt_token_service.go)、[认证用例测试](../../backend/internal/modules/identity/auth/auth_test.go) |
| HTTP 与内部业务身份 | [HTTP 认证边界](../../backend/internal/transport/rest/auth_middleware.go)、[内容授权适配](../../backend/internal/platform/authentication/context.go)、[BlogContent 身份](../../backend/internal/transport/grpc/blogcontent/server.go) |
| 页面、恢复与跨 tab | [登录页](../../frontend/src/views/public/LoginView.vue)、[会话控制器](../../frontend/src/services/auth-session.ts)、[应用装配](../../frontend/src/application.ts)、[路由守卫](../../frontend/src/router/guards.ts) |
| 请求认证域和旧响应保护 | [认证策略](../../frontend/src/request/auth-policy.ts)、[HTTP 通道](../../frontend/src/request/http.ts) |
| 消费者行为验证 | [认证域单元测试](../../frontend/tests/unit/ai-auth-policy.test.ts)、[生产认证 Edge 场景](../../frontend/tests/e2e/t13/auth-session.spec.ts)、[后端 HTTP 认证测试](../../backend/internal/transport/rest/auth_middleware_test.go) |

操作配置、启动与部署分别见[前端运行手册](../services/frontend/operations.md)和[后端开发指南](../services/backend/guides/backend-development.md)。本文说明现行合同与流程，不把测试入口存在写成本轮已执行结果。
