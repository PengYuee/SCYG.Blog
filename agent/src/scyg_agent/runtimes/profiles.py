"""冻结运行时能力、资源边界和工具权限画像."""

from dataclasses import dataclass
from enum import StrEnum
from typing import Final, override

from scyg_agent.domain.runs import RuntimeKind, RuntimeSelection, TaskType

RUNTIME_VERSION_V1: Final = "v1"
MIN_RECURSION_LIMIT: Final = 1
MAX_RECURSION_LIMIT: Final = 128
MIN_CONTEXT_TOKENS: Final = 1024
MAX_CONTEXT_TOKENS: Final = 1_000_000
MIN_TIMEOUT_SECONDS: Final = 1
MAX_TIMEOUT_SECONDS: Final = 3600


class ProfileRule(StrEnum):
    """关闭运行时画像构造失败的规则集合."""

    RECURSION_BOUNDS = "递归上限必须在 1 到 128 之间"
    CONTEXT_BOUNDS = "上下文令牌必须在 1024 到 1000000 之间"
    TIMEOUT_BOUNDS = "超时秒数必须在 1 到 3600 之间"
    TOOL_CAPABILITY = "工具能力与权限集合必须一致"
    TOOL_PERMISSION_CONTAINER = "工具权限必须是精确 frozenset"
    TOOL_PERMISSION_MEMBER = "工具权限必须属于闭集 ToolPermission"
    CAPABILITIES_TYPE = "capabilities 必须是精确 RuntimeCapabilities"
    BOUNDS_TYPE = "bounds 必须是精确 RuntimeBounds"
    SELECTION_TYPE = "selection 必须是精确 RuntimeSelection"
    BOOLEAN_CAPABILITY = "能力字段必须是精确 bool"
    INTEGER_BOUND = "资源边界字段必须是精确 int"


class ToolPermission(StrEnum):
    """声明深度运行时可调用的闭集 Blog 工具."""

    GET_PUBLISHED_ARTICLE = "get_published_article"
    SEARCH_ARTICLES = "search_articles"
    CREATE_ARTICLE_DRAFT = "create_article_draft"
    UPDATE_ARTICLE_DRAFT = "update_article_draft"
    PUBLISH_ARTICLE = "publish_article"
    ADD_ARTICLE_TAGS = "add_article_tags"


@dataclass(frozen=True, slots=True)
class InvalidRuntimeProfileError(ValueError):
    """报告不满足有界运行时画像契约的规则."""

    # rule 是稳定的内部规则名称, 不包含秘密或框架值.
    rule: ProfileRule

    @override
    def __str__(self) -> str:
        """返回稳定的中文画像错误."""
        return f"运行时画像无效: {self.rule}"


@dataclass(frozen=True, slots=True)
class RuntimeCapabilities:
    """以闭集布尔值声明适配器可提供的运行时能力."""

    # streaming 表示适配器可异步产出应用事件。
    streaming: bool
    # tools 表示画像允许调用明确授权的工具。
    tools: bool
    # interrupts 表示画像支持持久化人工中断。
    interrupts: bool
    # checkpoints 表示画像支持执行状态检查点。
    checkpoints: bool
    # subagents 表示画像允许固定子代理协作。
    subagents: bool

    def __post_init__(self) -> None:
        """拒绝布尔字段的整数或可变冒充值."""
        values = (
            self.streaming,
            self.tools,
            self.interrupts,
            self.checkpoints,
            self.subagents,
        )
        if any(type(value) is not bool for value in values):
            raise InvalidRuntimeProfileError(ProfileRule.BOOLEAN_CAPABILITY)


@dataclass(frozen=True, slots=True)
class RuntimeBounds:
    """限制单次运行的递归、上下文和墙钟时间."""

    # recursion_limit 限制递归或图步数。
    recursion_limit: int
    # context_tokens 限制传递给模型的上下文令牌数。
    context_tokens: int
    # timeout_seconds 限制一次执行或恢复的墙钟秒数。
    timeout_seconds: int

    def __post_init__(self) -> None:
        """在适配器启动前拒绝无界或无意义的资源限制."""
        values = (self.recursion_limit, self.context_tokens, self.timeout_seconds)
        if any(type(value) is not int for value in values):
            raise InvalidRuntimeProfileError(ProfileRule.INTEGER_BOUND)
        if not MIN_RECURSION_LIMIT <= self.recursion_limit <= MAX_RECURSION_LIMIT:
            raise InvalidRuntimeProfileError(ProfileRule.RECURSION_BOUNDS)
        if not MIN_CONTEXT_TOKENS <= self.context_tokens <= MAX_CONTEXT_TOKENS:
            raise InvalidRuntimeProfileError(ProfileRule.CONTEXT_BOUNDS)
        if not MIN_TIMEOUT_SECONDS <= self.timeout_seconds <= MAX_TIMEOUT_SECONDS:
            raise InvalidRuntimeProfileError(ProfileRule.TIMEOUT_BOUNDS)


@dataclass(frozen=True, slots=True)
class RuntimeProfile:
    """绑定一个版本选择及其能力、资源和工具权限."""

    # selection 是写入 Run 后不可变的运行时选择。
    selection: RuntimeSelection
    # capabilities 是适配器必须满足的闭集能力。
    capabilities: RuntimeCapabilities
    # bounds 是适配器必须执行的资源边界。
    bounds: RuntimeBounds
    # tool_permissions 是静态允许的 Blog 工具闭集。
    tool_permissions: frozenset[ToolPermission]

    def __post_init__(self) -> None:
        """保持工具能力声明和权限集合一致."""
        if type(self.selection) is not RuntimeSelection:
            raise InvalidRuntimeProfileError(ProfileRule.SELECTION_TYPE)
        if type(self.capabilities) is not RuntimeCapabilities:
            raise InvalidRuntimeProfileError(ProfileRule.CAPABILITIES_TYPE)
        if type(self.bounds) is not RuntimeBounds:
            raise InvalidRuntimeProfileError(ProfileRule.BOUNDS_TYPE)
        if type(self.tool_permissions) is not frozenset:
            raise InvalidRuntimeProfileError(ProfileRule.TOOL_PERMISSION_CONTAINER)
        if any(type(permission) is not ToolPermission for permission in self.tool_permissions):
            raise InvalidRuntimeProfileError(ProfileRule.TOOL_PERMISSION_MEMBER)
        if self.capabilities.tools != bool(self.tool_permissions):
            raise InvalidRuntimeProfileError(ProfileRule.TOOL_CAPABILITY)


SIMPLE_V1_PROFILE: Final = RuntimeProfile(
    selection=RuntimeSelection(RuntimeKind.SIMPLE, RUNTIME_VERSION_V1),
    capabilities=RuntimeCapabilities(
        streaming=True,
        tools=False,
        interrupts=False,
        checkpoints=False,
        subagents=False,
    ),
    bounds=RuntimeBounds(recursion_limit=4, context_tokens=32_768, timeout_seconds=120),
    tool_permissions=frozenset(),
)

COMPOSE_DEEP_V1_PROFILE: Final = RuntimeProfile(
    selection=RuntimeSelection(RuntimeKind.DEEP, RUNTIME_VERSION_V1),
    capabilities=RuntimeCapabilities(
        streaming=True,
        tools=True,
        interrupts=True,
        checkpoints=True,
        subagents=True,
    ),
    bounds=RuntimeBounds(recursion_limit=64, context_tokens=131_072, timeout_seconds=900),
    tool_permissions=frozenset(
        {
            ToolPermission.GET_PUBLISHED_ARTICLE,
            ToolPermission.SEARCH_ARTICLES,
            ToolPermission.CREATE_ARTICLE_DRAFT,
        }
    ),
)

RESEARCH_DEEP_V1_PROFILE: Final = RuntimeProfile(
    selection=RuntimeSelection(RuntimeKind.DEEP, RUNTIME_VERSION_V1),
    capabilities=COMPOSE_DEEP_V1_PROFILE.capabilities,
    bounds=COMPOSE_DEEP_V1_PROFILE.bounds,
    tool_permissions=frozenset(
        {
            ToolPermission.GET_PUBLISHED_ARTICLE,
            ToolPermission.SEARCH_ARTICLES,
        }
    ),
)

REVISE_DEEP_V1_PROFILE: Final = RuntimeProfile(
    selection=RuntimeSelection(RuntimeKind.DEEP, RUNTIME_VERSION_V1),
    capabilities=COMPOSE_DEEP_V1_PROFILE.capabilities,
    bounds=COMPOSE_DEEP_V1_PROFILE.bounds,
    tool_permissions=frozenset(
        {
            ToolPermission.GET_PUBLISHED_ARTICLE,
            ToolPermission.SEARCH_ARTICLES,
            ToolPermission.UPDATE_ARTICLE_DRAFT,
            ToolPermission.ADD_ARTICLE_TAGS,
        }
    ),
)

DEEP_V1_PROFILE: Final = COMPOSE_DEEP_V1_PROFILE


def deep_profile_for_task(task_type: TaskType) -> RuntimeProfile | None:
    """从 T14 唯一静态目录返回 DEEP 任务画像."""
    match task_type:  # noqa: RUF100  # noqa: MATCH_OK - TaskType 六个分支已完整映射。
        case TaskType.COMPOSE:
            return COMPOSE_DEEP_V1_PROFILE
        case TaskType.RESEARCH:
            return RESEARCH_DEEP_V1_PROFILE
        case TaskType.REVISE:
            return REVISE_DEEP_V1_PROFILE
        case TaskType.SUMMARY | TaskType.QUESTION | TaskType.POLISH:
            return None
