# 分类与标签：组织文章和维护分类配图

本文面向作者及内容模块维护者，说明如何维护文章使用的分类／标签、为什么公共筛选项与管理列表不同，以及何时允许删除。文章发布与版本规则见[文章](articles.md)，正文图片上传与绑定见[正文图片](article-images.md)。完整 HTTP 字段与路径以 [OpenAPI](../../backend/api/openapi.yaml) 为准。

## 建立并使用一个分类或标签

1. [登录作者会话](authentication.md)，进入 `/author/taxonomy`，或在文章编辑器的分类／标签设置中维护字典。
2. 创建分类或标签并填写名称。分类用于一篇文章的主归类，每篇文章必须选择一个；标签用于附加检索维度，可以不选，也可以选多个不同标签。两者是全站共享字典，不是某位作者的私有空间。
3. 在文章编辑器中选择需要的分类／标签，再保存文章。浏览器草稿里的选择不等于数据库已有文章引用，只有文章保存成功才建立或替换关联。
4. 修改名称使用当前资源版本。改名不改变 ID，不需要逐篇迁移文章；后续读取通过现有分类或标签取得新名称。分类的图片和菜单值由管理 API 创建／PATCH，当前作者页面只提供名称管理，不能把 API 能力当作页面已有配图编辑器。
5. 删除前，先迁走仍在使用的分类，或从文章中去除待删标签；只有不存在未删除文章引用时才能删除。删除后没有业务恢复入口。

作者页面创建分类时发送 `image: null`，并以当前分类选项数量加一设置 `menu`；这只是当前 UI 的创建策略，不是服务端保证的全局连续编号。文章编辑器创建成功后会选中新资源；它会协调刷新结果，避免一次陈旧刷新把刚创建的分类／标签丢掉。删除成功时仅清除受影响的本地选择，不替作者选另一个分类。

页面入口见 [TaxonomyView.vue](../../frontend/src/views/author/TaxonomyView.vue)、[ArticleEditorView.vue](../../frontend/src/views/author/ArticleEditorView.vue) 与 [use-article-tag-management.ts](../../frontend/src/composables/use-article-tag-management.ts)；真实管理请求和版本传递见 [article-type.ts](../../frontend/src/request/api/article-type.ts)、[tag.ts](../../frontend/src/request/api/tag.ts)。

## 公共筛选投影与管理字典不是同一份列表

| 入口 | 收录条件与内容 | 面向的流程 |
| --- | --- | --- |
| `/api/v1/article-types`、`/api/v1/tags` 及详情 | 字典项未删除，且至少有一篇已发布、未删除文章引用；返回公开文章数，分类还返回可空配图 | 访客筛选已公开文章 |
| `/api/v1/manage/article-types`、`/api/v1/manage/tags` 及详情 | 所有未删除字典项，包括从未使用、仅供草稿使用的项目；返回版本等管理信息，分类另有 `menu` | 作者创建、选择、修改、删除 |
| BlogContent `ListArticleTypes`／`ListTags` | 活动作者账号及管理授权后的管理字典 | 内部作者能力；不是匿名公共工具 |

公开 `articleCount` 只统计 `status = published` 且未删除的文章。只有草稿、只有归档文章、只有已删除文章或完全未使用的分类／标签，不以 `articleCount: 0` 暴露在公开列表中；公开详情也返回 `404`。归档或软删除最后一篇公开文章可以使对应筛选项消失，但不删除字典本身。

公开列表按文章数降序、ID 升序排序；名称查询去首尾空白后做不区分大小写的包含匹配，分页参数必须显式提供，每页最多 100 条。管理列表允许名称查询，使用契约规定的创建时间／更新时间／名称排序，不按公开文章数排序，也不保证按 `menu` 排序。前端管理字典适配器会逐页读完管理结果，不能误用只含公开项的列表作为编辑选择全集。

公开资源不返回管理版本字段和菜单值；公开详情 ETag 来自该字典项自身版本，文章引用数量变化不等于一次字典编辑。不要把公开计数理解成持久写入分类／标签的计数列。

公开投影见 [public_query_repository.go](../../backend/internal/modules/content/taxonomy/public_query_repository.go) 的 `publicArticleTypeQuery`、`publicTagQuery`，管理查询见 [article_type_query_repository.go](../../backend/internal/modules/content/taxonomy/article_type_query_repository.go)、[tag_query_repository.go](../../backend/internal/modules/content/taxonomy/tag_query_repository.go)。HTTP 公开／管理分流见 [article_type_handler.go](../../backend/internal/transport/rest/content/article_type_handler.go)、[tag_handler.go](../../backend/internal/transport/rest/content/tag_handler.go)；内部管理授权入口见 [management_query_service.go](../../backend/internal/modules/content/taxonomy/management_query_service.go)。

默认 HTTP 管理接口要求作者认证；当前真正装配的 `ContextAuthorizer` 只验证上下文中有 Principal，不检查角色、action、resource 或文章 owner。因此这些字典没有按作者分区，不能从“作者工作区”推导私有分类权限。通用身份边界见[作者登录与会话](authentication.md)。

## 名称、menu 与图片的含义

### 名称与重名

分类、标签名称使用同一校验：去首尾空白后不能为空，最多 60 个 Unicode 字符，必须是合法 UTF-8 且不含控制字符。名称保留输入大小写，不做统一小写或同义词归并。

未删除的分类名称在分类表内唯一，未删除的标签名称在标签表内唯一；分类和标签之间没有共享的名称唯一约束。唯一性由数据库索引保障，重名创建或改名返回 `409`。当前索引针对保存的 `name` 值，不是 `lower(name)` 索引；不要把搜索时的大小写不敏感匹配写成“名称大小写不敏感去重”。软删除后相同名称可以重新创建，但得到的是新资源，不是恢复旧 ID。

校验见 [article_type_tag_validation.go](../../backend/internal/modules/content/taxonomy/article_type_tag_validation.go) 的 `parseName`，索引见 [000001_initial.up.sql](../../backend/migrations/000001_initial.up.sql) 的 `article_types_name_active_idx`、`tags_name_active_idx`。

### menu 是数值，不是分类层级或路由开关

HTTP 字段使用 `menu`，当前内部持久化名称为 `meun`。它表示非负菜单排序值；领域与 smallint 存储约束把实际允许范围限制在 `0` 至 `32767`。OpenAPI 声明非负值，但没有把该存储上限写成 maximum；调用方仍须遵守当前实现范围。

`menu` 不表达父子分类，不代表“是否公开”，也不保证唯一、连续或固定排序方向。公开列表按文章计数排序，管理列表使用显式 `sort` 白名单；不要通过调整 `menu` 期待这两个接口自动换序。

### 分类配图不是文章封面或正文图片

分类 `image` 是可空地址字符串。同一分类下的文章卡片共享分类配图；文章资源没有独立封面字段，也不会从第一张正文图片自动生成封面。文章响应包含分类 ID、名称和图片摘要，使调用方能识别文章所属分类；正文图片的上传、归属、绑定和清理是另一条[图片业务流程](article-images.md)。

图片在创建时可不设置或为 `null`。PATCH 省略 `image` 保持原值，显式 `image: null` 清空地址，字符串替换地址。服务端只检查 UTF-8、最多 512 个 Unicode 字符和无控制字符，不抓取文件、不检测可达性、不承诺地址已经存在，也不创建受管正文图片记录；当前也不做 HTTP(S) URL 格式校验或将空字符串归一化成 `null`。

前端分类字典适配器另有图片 URL 边界：相对路径相对于配置的 Blog `serverUrl` 解析，绝对地址仅接受 HTTP(S)，拒绝 `//` 协议相对地址及其他协议。这是展示／解析约束，不是后端保存地址时已验证的保证。非法地址可能使适配器报解析失败，不能把它与“有效地址但图片加载失败”混为一谈。

卡片仅在有图片地址且加载未失败时展示图片；未配置和加载失败都会落到同一个图片占位。看到占位不能反推分类一定未配置图片；应先检查 API 的 `image`，再检查浏览器请求。相关实现见 [article_type_service.go](../../backend/internal/modules/content/taxonomy/article_type_service.go) 的 `PatchArticleType`、[article_type_tag_validation.go](../../backend/internal/modules/content/taxonomy/article_type_tag_validation.go) 的 `parseImage`／`parseMeun`、[api.ts](../../frontend/src/types/api.ts) 的 `normalizeImageUrl` 和 [ArticleCard.vue](../../frontend/src/components/article/ArticleCard.vue)。

## 版本、删除限制与并发引用

分类和标签创建版本为 `1`，实际修改后版本递增。分类 PATCH 至少提供名称、图片或菜单值之一；标签 PATCH 用于改名。以正确版本提交相同值，不会产生一次新修订，而是 `409` 无变化冲突。

HTTP 修改和删除都要求强 `If-Match`，例如 `"2"`。缺失返回 `428`，非法格式返回 `400`，过期返回 `412`。成功修改返回最新资源和 ETag，调用方应替换手中版本；删除成功返回 `204`。删除是软删除，后续正常管理与公开查询均不返回该资源，再次串行删除通常得到 `404`，不是重复成功。

删除判断看的是**所有未删除文章**，不只是公开计数：

- 草稿、已发布、已归档文章都能阻止删除其分类或标签。公开计数为零或公开列表看不到某项，不代表可以删除。
- 分类删除前必须没有未删除文章的 `article_type_id` 指向它；标签删除前必须没有关联到未删除文章的 `article_tags` 引用。
- 已软删除文章保留的关联不阻止分类／标签删除。此规则只用于业务软删除检查，不意味着数据库物理外键已解除。
- 归档文章仍然阻止删除，而当前归档文章不能修订、删除或恢复。因此不能承诺“把归档文章先移走分类／去掉标签即可删除字典”；这些归档引用在现行业务中会继续保留。

删除与文章并发保存不能只靠页面先查询一次引用数。taxonomy 修改／删除在事务内锁住目标字典行、核对版本，删除前再检查引用；文章保存则对要引用的未删除分类／标签持共享行锁，验证存在后才保存文章和关联。两条路径互相排斥：保存先提交的引用会让删除失败，删除先提交则后续文章保存不能引用已删除项。标签集合按 ID 排序锁定，分类也按 ID 排序锁定，保护由后端正常业务事务承担，不由浏览器控件承担。

规则见 [article_type_service.go](../../backend/internal/modules/content/taxonomy/article_type_service.go)、[tag_service.go](../../backend/internal/modules/content/taxonomy/tag_service.go)；删除引用检查和锁见 [article_type_repository.go](../../backend/internal/modules/content/taxonomy/article_type_repository.go) 的 `findArticleType`／`updateArticleType`、[tag_repository.go](../../backend/internal/modules/content/taxonomy/tag_repository.go) 的 `findTag`／`updateTag`；文章侧共享锁见 [article_repository.go](../../backend/internal/modules/content/article/article_repository.go) 的 `lockArticleTypes`／`lockTags`。它们保护正常应用路径，不把绕过业务服务的直接 SQL 写入描述成等价流程。

## 失败后处理与数据归属

后端 taxonomy feature 拥有 `article_types`、`tags` 的写入及公开／管理查询；文章 feature 拥有 `articles`、`article_tags` 的内容和关联写入。文章与分类摘要的跨 feature 一致性由应用层 `ArticleResponses` 协调，详见[文章](articles.md)。前端持有选项、选择值和版本，不能用本地删选项代替服务端删除，也不能把保存前的选择当作已建立引用。

| 失败 | 应如何处理 |
| --- | --- |
| `400` | 修正名称、ID、图片字符串、菜单值、PATCH 或 ETag；图片地址保存通过不代表浏览器一定能加载 |
| `401`／`403` | 恢复合法作者会话或处理授权失败；字典是共享的，但不是匿名可写的 |
| `404` | 重新读取管理字典，确认是否已删除；公开入口还可能只是没有公开文章引用 |
| `409` | 检查重名、无变化或文章引用限制；有引用时先按文章允许的状态处理引用，不能强行删字典 |
| `412`／`428` | 取得当前版本，重新确认后再修改／删除，不能静默替换版本并重放破坏操作 |
| 网络失败或 `500` | 先读回确认当前资源；字典 HTTP 写入没有成功重放 key，不能把客户端失败等同于没执行 |

文章编辑器的标签删除失败会保留标签与选择；版本过期时刷新目标版本，要求下一次操作基于最新状态。分类名称修改在编辑器和字典页也有 `404`／`412` 刷新协调。不同页面的错误交互实现不完全相同，不应把某个编辑器的冲突提示概括成所有字典操作均自动恢复。服务端的版本和引用限制始终是最终结果来源。HTTP 稳定错误映射见 [problem.go](../../backend/internal/transport/rest/content/problem.go)。

## 源码与验证入口

以下入口供维护者定位和验证，本文不宣称本次已运行这些测试：

- [taxonomy_integration_test.go](../../backend/internal/modules/content/taxonomy/taxonomy_integration_test.go)：`Test_TaxonomyService_Postgres_preservesConstraintsAndDeletionRules` 覆盖重名、引用限制、软删除文章不再阻止删除；`Test_TaxonomyService_Postgres_publicQueriesCountOnlyPublishedArticles` 覆盖公开计数与可见范围。
- [taxonomy_test.go](../../backend/internal/modules/content/taxonomy/taxonomy_test.go)：排序白名单、数据映射及数据库错误分类。
- [article_type_handler_test.go](../../backend/internal/transport/rest/content/article_type_handler_test.go)、[public_query_handler_test.go](../../backend/internal/transport/rest/content/public_query_handler_test.go)：管理分类响应、图片 PATCH presence 和公共查询契约。
- [article_responses_integration_test.go](../../backend/internal/modules/content/application/article_responses_integration_test.go)：文章分类摘要装配及失败回滚；分类／标签引用的并发保护具体实现以上述 repository 共享锁／更新锁为准，不能把顺序集成用例说成已覆盖全部并发时序。
- [article-editor-tag-management.test.ts](../../frontend/tests/component/article-editor-tag-management.test.ts)：创建刷新协调、删除失败保留选择、版本过期后使用新版本及重复交互门禁。
- [article-type-dropdown.test.ts](../../frontend/tests/component/article-type-dropdown.test.ts)、[article-settings.test.ts](../../frontend/tests/component/article-settings.test.ts)：编辑器分类与标签控件入口。执行环境和命令见[前端开发指南](../services/frontend/development.md)与[后端开发指南](../services/backend/guides/backend-development.md)。
