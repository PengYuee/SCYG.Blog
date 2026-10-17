# 公共 AI 搜索

## 业务范围

公共 AI 搜索让未登录访客用自然语言查找站内已发布文章，首页以普通文章卡片展示匹配结果。它与关键词筛选采用不同的检索方式，但共用列表界面；请求使用独立匿名身份，能力固定为 search，内容范围固定为 public。

普通文章查询见[文章管理](articles.md)；作者四能力及共同 Run 状态、快照、幂等和取消语义见 [AI 任务与作者辅助](ai-assistance.md)。本模块只维护公共搜索的身份、内容、额度与失败差异，不复制共同任务生命周期。

## 从提问到结果

1. 访客在首页切换到 AI 搜索，填写问题；最多参考文章数在“搜索选项”中调整。旧 `/ai/search` 会转到首页 AI 模式。即使浏览器已登录作者，公共搜索也不借用作者 JWT。
2. 提交前，浏览器取得或恢复当前 tab 的匿名搜索凭证。签发入口不携带作者 Bearer，也不接受请求体；成功响应带凭证期限及当前公共输入限制，前端按这些限制检查表单。
3. 浏览器以匿名 Bearer、新逻辑操作的 UUID v4 key 和输入创建 Run。Blog 校验输入与频率，固定能力和 public 范围；客户端和模型均不能覆盖它们。
4. Agent 优先处理既有成功 key 重放，再核对输入、公共接纳和额度，原子创建持久 Run。HTTP `202` 只表示请求已接纳，不代表模型成功，也不代表已经得到执行名额。
5. Worker 在有本地空槽及公共执行名额后认领任务；模型只通过获授权的只读工具访问公开文章。公开 SQL、执行授权与预算是最终约束，不依赖模型自觉遵守提示词。
6. 首页结果区先查询 Run，再为非终态建立事件流；通知、断线和 EOF 均回到快照判断。标题下以加载动画和短句说明执行状态，完成后替换为文章数量与提交主题，文章卡片通过现有公开接口读取真实字段。没有可展示文章不自动等于执行失败；结果引用后来不可公开读取时不伪造卡片。公共界面不展开结论、证据、任务标识和活动日志，Run 中的原始结果保持不变。模式切换保留草稿和当前任务，不会取消服务端执行；取消需要显式确认。旧任务地址仍转到首页相应结果，失败、取消与明确发起新操作相互区分。

HTTP 字段、请求大小、认证方案和状态码以 [OpenAPI](../../backend/api/openapi.yaml) 的公共 session/search 及共同 Run 操作为准。具体可变限制维护在[Blog 配置模板](../../backend/config.example.yaml)与 [Agent 公共配置](../../agent/agent.toml.example)，不在页面或本文另设一套数值。

## 匿名身份和恢复边界

匿名凭证只用于创建公共搜索，以及该主体自己的 Run 查询、事件订阅和取消，不能用于作者 AI、Resume 或文章管理。它在当前 tab 的 sessionStorage 中保存；浏览器不解码它来判断权限，不放入 URL、Cookie 或普通日志。

- 作者登录与匿名身份同时存在，但相互隔离。匿名 `401` 不登出作者；作者退出或切换账号也不能将旧作者 Run 转成匿名任务。
- Run 的存在性、owner 和冻结内容范围由服务端核对，不存在和不可访问统一返回 `404`。知道一个 Run 地址不代表可以读取它。
- 匿名凭证丢失或过期后，新凭证是新主体，不能恢复原身份。旧未知操作不能静默换凭证重建，否则可能重复执行并产生费用。
- sessionStorage 不可用时，本页可使用内存中的真实凭证，但刷新后不能承诺恢复。页面应明确要求重新开始，而不是伪造持久身份。

作者登录机制见[登录与会话](authentication.md)。匿名主体不是注册用户，也不是可靠的永久访客身份；IP 只用于频率控制，不作为 Run owner。不能把换凭证、换 tab 或新建 Run 理解为获得作者权限。

## 公开内容的保证和局限

public 读取复用已发布且未删除的文章投影，只允许 BlogContent 的 SearchArticles/GetArticle；不会把匿名主体装进作者 Principal 后调用管理读取。输出引用必须来自本 Run 已成功授权读取的文章。草稿、归档和删除规则由[文章模块](articles.md)维护，工具参数不能放宽这些条件。

搜索工具向模型说明状态数字的实际含义；公共请求只允许不按状态筛选或明确选择已发布。不筛选也仍是公开投影，不等于读取全部文章。模型提交不允许的筛选会被拒绝，Schema 说明不替代服务端权限。

恢复公共 checkpoint 时，继续执行前重新检查其持久来源是否仍公开；只验证来源身份与已发布状态，丢弃新读回正文，不把“重新授权”当作新一轮模型检索或无限预算。来源失效就终止恢复，不能沿用已经私有的来源继续生成。

已经持久化的 Run 结果是执行时的结果快照，不是文章的实时镜像。共同快照读取按 owner/范围返回既有结果，不能据此承诺文章后来撤回时自动清除所有历史引用或已交付文本；来源链接点击时仍受当前文章读取规则限制。

## 接纳、频率、日额度和执行名额

这些控制解决不同问题，不能用“排队成功”替代执行保证：

| 控制 | 当前业务含义 |
| --- | --- |
| 公共接纳 | 同一 Agent 数据库中 public 非终态 Run 的固定上限为 10，通过创建事务仲裁后才能插入。不是十个任务同时运行，也不是可调的 admission_capacity 配置。 |
| IP 频率 | 匿名凭证签发、公共创建及轻量操作分别受共享频率约束；同部署 Blog 副本使用相同桶。换匿名凭证不重置同一 IP 的频率限制。 |
| UTC 日额度 | 同一 Agent 公共控制命名空间按 UTC 自然日共用创建次数；不是每个 tab 或作者账号各自一份额度。owner 与规范化 key 用来识别同一预占操作，不是另建每日计数维度。 |
| 公共执行名额 | Worker 取得共享 Redis token 后才认领 public，不把仅有队列名额的任务提前标为 running，也不让公共等待占住全部作者槽。 |
| 单次执行预算 | 问题与结果数量、模型输出、图步数、每次尝试的工具次数和读取量、首次执行起的总期限共同限制一次任务，不由浏览器或模型放宽。 |
| SSE 连接名额 | 按当前匿名 Run 约束跨副本的活动订阅；断开释放本连接名额，不等于取消 Run 或退回创建次数。 |

终态不占公共接纳容量，但不证明在途模型调用已经结束。公共执行 token 覆盖本地 runner 和子任务的实际生命周期，真正退出后才释放；数据库 lease 仍独立控制认领、续租和终态提交。author 不经过公共执行桶，不能把这些限制全局套到作者模型配置。

总执行截止时间在首次认领时持久保存，重认领不能延长；工具与读取预算属于一次执行尝试，重认领时可能重新开始。公共预算中止进入策略失败，不作为普通工具错误继续让模型无界执行。名额与取消约束本地有效持有者，不承诺网络分区等情况下远端 Provider 严格零重叠或立即停止计费。

## 创建次数如何扣除与补偿

公共创建在确认不是成功重放、输入有效且具备接纳条件后，以 owner/key 原子预占日额度，再提交数据库创建。成功操作接续不重复扣次数；已成功创建 Run 后的模型失败或用户取消不退创建次数。

仅在能够证明创建未提交、事务已回滚时，才按原 UTC 日和当前操作 token 退款一次；旧 token 不能退掉新尝试。数据库提交开始后的结果不明、断线或笼统错误不作为退款证明；提交成功后的消费标记失败也不退款。跨午夜的退款回到原日，不给今天增加额外额度。

额度记录的正常重启保留依赖 Redis AOF、持久卷及 noeviction；它不是财务账本，不保证机器损坏或数据丢失时的零损失，也没有额外后台退款队列或分布式对账平台。运行前提及配置检查见 [Agent 公共搜索运行控制](../services/agent/public-search.md)。

## 主要拒绝、故障和用户动作

| 结果 | 正确处理 |
| --- | --- |
| 输入错误、请求超大或媒体类型不符 | 保留可修改输入，按 API 返回修正；不要伪造成功 Run。 |
| `401` | 只失效匿名域；凭证过期或丢失不能用新身份恢复旧任务。 |
| `404` | 任务不存在、owner/范围不允许，或相关集成未启用；不向访客披露私有任务是否存在。 |
| `409` | 查询权威快照，不覆盖已处理决定或重建结果未知的请求；公共主体本来就不能 Resume。 |
| `429` | 频率、连接、接纳或日额度不足。只使用服务端可确定的等待秒数；等待后恢复手动重试，不自动创建新操作或保证届时一定有执行名额。 |
| `503` | 依赖不可用，保留原操作与 Run 标识，不降级成不受额度约束的搜索。 |
| Run 失败 | 以简短说明报告失败，并保留已返回且仍可公开读取的文章；再次执行需要用户明确发起新操作，不改写旧 Run 或伪称自动重放成功。 |

Redis 不可用时，新匿名凭证、新公共创建与新 SSE 拒绝为 `503`；既有 Run GET/Cancel 可以暂时跳过轻量频率保护，但不跳过认证、owner 或用途校验。旧执行/订阅失去续租时停止本地活动。运行故障的取证与恢复步骤由服务手册维护。

未知创建、取消与游标重连沿 [AI 任务](ai-assistance.md) 的共同规则：保留原身份、原 key 与原输入，同操作重试不等于用户明确开始新任务。

## 服务、数据与维护入口

Blog 拥有匿名凭证、浏览器认证边界、IP 桶与 SSE 连接控制；Agent 拥有公共接纳、Run/结果/持久事件、额度预占与执行策略；Blog 内容数据库拥有文章是否公开的事实。Redis 名额不是 Run 状态，LangGraph checkpoint 也不是事件或结果查询接口。

| 核对对象 | 权威实现与验证入口 |
| --- | --- |
| HTTP 与独立凭证 | [公共 handler](../../backend/internal/transport/rest/public_search_handler.go)、[认证边界](../../backend/internal/transport/rest/auth_middleware.go)、[匿名签发验证](../../backend/internal/platform/authentication/public_search_token.go)、[认证测试](../../backend/internal/transport/rest/public_search_auth_test.go) |
| 公开内容读取 | [BlogContent 公开读取](../../backend/internal/transport/grpc/blogcontent/read_service.go)、[可信身份](../../backend/internal/transport/grpc/blogcontent/server.go)、[内容 RPC 集成测试](../../backend/internal/modules/content/application/blog_content_grpc_integration_test.go) |
| 创建、成功重放与接纳 | [控制应用](../../agent/src/scyg_agent/application/control.py)、[控制持久测试](../../agent/tests/application/test_control_postgresql.py) |
| 日额度与执行 token | [Redis controls](../../agent/src/scyg_agent/adapters/redis/public_controls.py)、[控制测试](../../agent/tests/adapters/redis/test_public_controls.py) |
| 实际模型、工具与恢复预算 | [生产图](../../agent/src/scyg_agent/agents/production.py)、[只读网关](../../agent/src/scyg_agent/agents/tool_gateway.py)、[预算对象](../../agent/src/scyg_agent/agents/public_policy.py)、[公共恢复测试](../../agent/tests/agents/test_public_recovery.py) |
| 浏览器恢复与展示 | [匿名会话](../../frontend/src/services/anonymous-ai-session.ts)、[Run 生命周期](../../frontend/src/services/ai-run.ts)、[Edge 场景](../../frontend/tests/e2e/ai-workflows.spec.ts) |
| 配置和分布式运行 | [服务运行控制](../services/agent/public-search.md)、[Blog Redis 控制实现及集成测试](../../backend/internal/platform/publicsearch/)、[根 Compose](../../compose.yaml) |

上述是本模块维护时应核对的入口，不表示一次文档整理重新执行了全部分布式、模型或浏览器验证。
